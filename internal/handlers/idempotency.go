package handlers

import (
	"bytes"
	"counter/internal/contract"
	"counter/internal/database"
	"counter/internal/middleware"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
)

type idempotencyContextKey struct{}

// IdempotentMutation adds optional, durable replay to non-counter mutations.
// Authentication and route authorization must run before this wrapper.
// Counter increment/set retain their existing operation-history contract.
func IdempotentMutation(db *database.DB, credentialResponse bool, next fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		key := strings.TrimSpace(string(ctx.Request.Header.Peek("Idempotency-Key")))
		if key == "" {
			next(ctx)
			return
		}
		if err := contract.ValidateIdempotencyKey(contract.V1, key); err != nil {
			respondWithError(ctx, fasthttp.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", err.Error())
			return
		}
		key = uuid.MustParse(key).String()
		scope := principalID(ctx) + ":" + string(ctx.Method()) + ":" + string(ctx.Path())
		requestHash := idempotencyRequestHash(ctx)
		requestContext := requestDatabaseContext(ctx)
		tx, err := db.BeginTxx(requestContext, nil)
		if err != nil {
			respondWithServiceError(ctx, err, fasthttp.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Idempotency storage is temporarily unavailable")
			return
		}
		defer tx.Rollback()
		// A transaction-scoped lock serializes the same request across replicas.
		// Unrelated keys proceed independently; hash collisions only add waiting.
		if _, err := tx.ExecContext(requestContext, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, scope+":"+key); err != nil {
			respondWithServiceError(ctx, err, fasthttp.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Idempotency storage is temporarily unavailable")
			return
		}
		if _, err := tx.ExecContext(requestContext, `DELETE FROM idempotency_requests WHERE scope = $1 AND request_key = $2 AND expires_at <= NOW()`, scope, key); err != nil {
			respondWithServiceError(ctx, err, fasthttp.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Idempotency storage is temporarily unavailable")
			return
		}
		var saved struct {
			Hash        []byte `db:"request_hash"`
			Status      int    `db:"status_code"`
			ContentType string `db:"content_type"`
			Body        []byte `db:"response_body"`
		}
		err = tx.GetContext(requestContext, &saved, `SELECT request_hash, status_code, content_type, response_body FROM idempotency_requests WHERE scope = $1 AND request_key = $2`, scope, key)
		if err == nil {
			if !bytes.Equal(saved.Hash, requestHash) {
				middleware.RecordEvent(ctx, "idempotency_conflict")
				respondWithError(ctx, fasthttp.StatusConflict, "IDEMPOTENCY_KEY_REUSED", "Idempotency key was reused with different request data")
				return
			}
			ctx.SetStatusCode(saved.Status)
			ctx.Response.Header.SetContentType(saved.ContentType)
			ctx.SetBody(saved.Body)
			ctx.Response.Header.Set("Idempotency-Replayed", "true")
			middleware.RecordEvent(ctx, "idempotency_replay")
			return
		}
		if !errors.Is(err, sql.ErrNoRows) {
			respondWithServiceError(ctx, err, fasthttp.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Idempotency storage is temporarily unavailable")
			return
		}

		ctx.SetUserValue(idempotencyContextKey{}, database.WithTransaction(requestContext, tx))
		defer ctx.RemoveUserValue(idempotencyContextKey{})
		next(ctx)
		if ctx.Response.StatusCode() < 200 || ctx.Response.StatusCode() >= 300 {
			return // Roll back both the mutation and reservation on any failure.
		}
		body := append([]byte{}, ctx.Response.Body()...)
		if credentialResponse {
			// Never persist issued secrets, including for the initial response.
			var response map[string]json.RawMessage
			if err := json.Unmarshal(body, &response); err != nil {
				respondWithError(ctx, fasthttp.StatusInternalServerError, "INTERNAL_ERROR", "Failed to prepare credential replay")
				return
			}
			delete(response, "api_key")
			body, err = json.Marshal(response)
			if err != nil {
				respondWithError(ctx, fasthttp.StatusInternalServerError, "INTERNAL_ERROR", "Failed to prepare credential replay")
				return
			}
		}
		_, err = tx.ExecContext(requestContext, `INSERT INTO idempotency_requests (scope, request_key, request_hash, status_code, content_type, response_body) VALUES ($1, $2, $3, $4, $5, $6)`, scope, key, requestHash, ctx.Response.StatusCode(), string(ctx.Response.Header.ContentType()), body)
		if err == nil {
			err = tx.Commit()
		}
		if err != nil {
			// Replace any response containing an uncommitted credential secret.
			respondWithServiceError(ctx, err, fasthttp.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Idempotency storage is temporarily unavailable")
			return
		}
		ctx.Response.Header.Set("Idempotency-Replayed", "false")
	}
}

func idempotencyRequestHash(ctx *fasthttp.RequestCtx) []byte {
	body := ctx.Request.Body()
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if json.Valid(body) {
		value, orderSensitive, err := replayJSONValue(decoder)
		if err == nil {
			if orderSensitive {
				// Duplicate fields and case aliases can be order-sensitive when
				// handlers decode into structs. Preserve them rather than merging.
				var compact bytes.Buffer
				if json.Compact(&compact, body) == nil {
					body = compact.Bytes()
				}
			} else if canonical, err := json.Marshal(value); err == nil {
				body = canonical
			}
		}
	}
	query := string(ctx.URI().QueryString())
	if parsed, err := url.ParseQuery(query); err == nil {
		query = parsed.Encode()
	}
	hash := sha256.New()
	hash.Write([]byte(query))
	hash.Write([]byte{0})
	hash.Write(body)
	return hash.Sum(nil)
}

// replayJSONValue canonicalizes unambiguous JSON objects while detecting keys
// whose order must be retained. Number tokens preserve int64 precision.
func replayJSONValue(decoder *json.Decoder) (interface{}, bool, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, false, err
	}
	delimiter, container := token.(json.Delim)
	if !container {
		return token, false, nil
	}
	orderSensitive := false
	if delimiter == '{' {
		object := make(map[string]interface{})
		seen := make(map[string]bool)
		for decoder.More() {
			name, err := decoder.Token()
			if err != nil {
				return nil, false, err
			}
			key := name.(string) // Input was validated by json.Valid.
			// Conservatively group Unicode case variants as well as ASCII.
			folded := strings.ToLower(strings.ToUpper(key))
			if seen[folded] {
				orderSensitive = true
			}
			seen[folded] = true
			value, sensitive, err := replayJSONValue(decoder)
			if err != nil {
				return nil, false, err
			}
			orderSensitive = orderSensitive || sensitive
			object[key] = value
		}
		_, err := decoder.Token()
		return object, orderSensitive, err
	}
	array := make([]interface{}, 0)
	for decoder.More() {
		value, sensitive, err := replayJSONValue(decoder)
		if err != nil {
			return nil, false, err
		}
		orderSensitive = orderSensitive || sensitive
		array = append(array, value)
	}
	_, err = decoder.Token()
	return array, orderSensitive, err
}
