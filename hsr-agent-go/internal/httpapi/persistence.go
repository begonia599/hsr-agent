package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"hsr-agent-go/internal/agent"

	"github.com/jackc/pgx/v5"
)

type persistentToolCall struct {
	Seq        int
	ToolCallID string
	Name       string
	Args       json.RawMessage
	Result     json.RawMessage
	Error      string
	LatencyMS  int64
}

type toolTraceCollector struct {
	calls   []*persistentToolCall
	byID    map[string]*persistentToolCall
	nextSeq int
}

func newToolTraceCollector() *toolTraceCollector {
	return &toolTraceCollector{byID: map[string]*persistentToolCall{}}
}

func (c *toolTraceCollector) Add(event agent.Event) {
	switch event.Type {
	case "tool_call":
		call := &persistentToolCall{
			Seq:        c.nextSeq,
			ToolCallID: event.ToolCallID,
			Name:       event.Name,
			Args:       validRawJSON(event.Args, `{}`),
		}
		c.nextSeq++
		c.calls = append(c.calls, call)
		if event.ToolCallID != "" {
			c.byID[event.ToolCallID] = call
		}
	case "tool_result":
		call := c.byID[event.ToolCallID]
		if call == nil {
			call = &persistentToolCall{
				Seq:        c.nextSeq,
				ToolCallID: event.ToolCallID,
				Name:       event.Name,
				Args:       json.RawMessage(`{}`),
			}
			c.nextSeq++
			c.calls = append(c.calls, call)
			if event.ToolCallID != "" {
				c.byID[event.ToolCallID] = call
			}
		}
		if call.Name == "" {
			call.Name = event.Name
		}
		call.Result = marshalRawJSON(event.Result, `null`)
		call.Error = event.Error
		call.LatencyMS = event.LatencyMS
	}
}

func (c *toolTraceCollector) Calls() []persistentToolCall {
	out := make([]persistentToolCall, 0, len(c.calls))
	for _, call := range c.calls {
		if call == nil {
			continue
		}
		if call.Args == nil {
			call.Args = json.RawMessage(`{}`)
		}
		if call.Result == nil {
			call.Result = json.RawMessage(`null`)
		}
		out = append(out, *call)
	}
	return out
}

func validRawJSON(raw json.RawMessage, fallback string) json.RawMessage {
	if len(raw) > 0 && json.Valid(raw) {
		return raw
	}
	return json.RawMessage(fallback)
}

func marshalRawJSON(value any, fallback string) json.RawMessage {
	if value == nil {
		return json.RawMessage(fallback)
	}
	data, err := json.Marshal(value)
	if err != nil || !json.Valid(data) {
		return json.RawMessage(fallback)
	}
	return data
}

func (s *Server) ensureConversation(ctx context.Context, conversationID int64, sessionID string, message string) (int64, error) {
	if s.db == nil {
		return 0, fmt.Errorf("database is not configured")
	}
	if conversationID > 0 {
		var id int64
		err := s.db.QueryRow(ctx, `SELECT id FROM conversations WHERE id = $1`, conversationID).Scan(&id)
		return id, err
	}
	title := conversationTitle(message)
	err := s.db.QueryRow(ctx, `
INSERT INTO conversations (session_id, title)
VALUES (NULLIF($1, ''), NULLIF($2, ''))
RETURNING id`, strings.TrimSpace(sessionID), title).Scan(&conversationID)
	return conversationID, err
}

func conversationTitle(message string) string {
	message = strings.Join(strings.Fields(strings.TrimSpace(message)), " ")
	runes := []rune(message)
	if len(runes) > 40 {
		return string(runes[:40])
	}
	return message
}

func (s *Server) insertMessage(ctx context.Context, conversationID int64, role string, content string, turnID *int64) (int64, error) {
	if s.db == nil || conversationID <= 0 {
		return 0, fmt.Errorf("database is not configured")
	}
	var id int64
	err := s.db.QueryRow(ctx, `
INSERT INTO messages (conversation_id, role, content, turn_id)
VALUES ($1, $2, $3, $4)
RETURNING id`, conversationID, role, content, turnID).Scan(&id)
	return id, err
}

func (s *Server) startAgentTurn(ctx context.Context, conversationID int64, traceID string, model string) (int64, error) {
	if s.db == nil || conversationID <= 0 {
		return 0, fmt.Errorf("database is not configured")
	}
	var id int64
	err := s.db.QueryRow(ctx, `
INSERT INTO agent_turns (conversation_id, trace_id, model, status)
VALUES ($1, $2, $3, 'running')
RETURNING id`, conversationID, traceID, model).Scan(&id)
	return id, err
}

func (s *Server) finishAgentTurn(ctx context.Context, turnID int64, conversationID int64, result agent.RunResult, status string, err error, started time.Time, calls []persistentToolCall) error {
	if s.db == nil || turnID <= 0 {
		return fmt.Errorf("database is not configured")
	}
	if status == "" {
		status = result.Status
	}
	if status == "" {
		status = "completed"
	}
	var errorMessage *string
	var errorCode *string
	if err != nil {
		if status == "" || status == "completed" {
			status = "error"
		}
		msg := err.Error()
		code := "LLM_UPSTREAM_ERROR"
		if status == "aborted" {
			code = "ABORTED"
		}
		errorMessage = &msg
		errorCode = &code
	}
	latencyMS := int(time.Since(started).Milliseconds())
	tx, txErr := s.db.Begin(ctx)
	if txErr != nil {
		return txErr
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback(ctx)
		}
	}()
	_, txErr = tx.Exec(ctx, `
UPDATE agent_turns
SET status = $2,
    finished_at = now(),
    latency_ms = $3,
    tool_call_count = $4,
    prompt_tokens = NULLIF($5, 0),
    completion_tokens = NULLIF($6, 0),
    total_tokens = NULLIF($7, 0),
    error_code = $8,
    error_message = $9,
    final_answer = NULLIF($10, '')
WHERE id = $1`, turnID, status, latencyMS, len(calls), result.Usage.PromptTokens, result.Usage.CompletionTokens, result.Usage.TotalTokens, errorCode, errorMessage, result.Message)
	if txErr != nil {
		return txErr
	}
	for _, call := range calls {
		name := call.Name
		if strings.TrimSpace(name) == "" {
			name = "unknown"
		}
		_, txErr = tx.Exec(ctx, `
INSERT INTO agent_tool_calls (turn_id, seq, tool_call_id, tool_name, arguments, result, error, latency_ms)
VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, NULLIF($7, ''), NULLIF($8, 0))
ON CONFLICT (turn_id, seq) DO UPDATE
SET tool_call_id = EXCLUDED.tool_call_id,
    tool_name = EXCLUDED.tool_name,
    arguments = EXCLUDED.arguments,
    result = EXCLUDED.result,
    error = EXCLUDED.error,
    latency_ms = EXCLUDED.latency_ms`, turnID, call.Seq, call.ToolCallID, name, call.Args, call.Result, call.Error, int(call.LatencyMS))
		if txErr != nil {
			return txErr
		}
	}
	if err == nil && strings.TrimSpace(result.Message) != "" {
		_, txErr = tx.Exec(ctx, `
INSERT INTO messages (conversation_id, role, content, turn_id)
VALUES ($1, 'assistant', $2, $3)`, conversationID, result.Message, turnID)
		if txErr != nil {
			return txErr
		}
	}
	_, txErr = tx.Exec(ctx, `UPDATE conversations SET updated_at = now() WHERE id = $1`, conversationID)
	if txErr != nil {
		return txErr
	}
	if txErr = tx.Commit(ctx); txErr != nil {
		return txErr
	}
	tx = nil
	return nil
}

func (s *Server) failAgentTurn(ctx context.Context, turnID int64, started time.Time, err error) {
	if s.db == nil || turnID <= 0 || err == nil {
		return
	}
	message := err.Error()
	_, _ = s.db.Exec(ctx, `
UPDATE agent_turns
SET status = 'error',
    finished_at = now(),
    latency_ms = $2,
    error_code = 'LLM_UPSTREAM_ERROR',
    error_message = $3
WHERE id = $1`, turnID, int(time.Since(started).Milliseconds()), message)
}

func (s *Server) getConversation(ctx context.Context, id int64) (map[string]any, error) {
	var row struct {
		ID        int64
		SessionID *string
		UserID    *string
		Title     *string
		CreatedAt time.Time
		UpdatedAt time.Time
		Meta      json.RawMessage
	}
	err := s.db.QueryRow(ctx, `
SELECT id, session_id, user_id, title, created_at, updated_at, meta
FROM conversations
WHERE id = $1`, id).Scan(&row.ID, &row.SessionID, &row.UserID, &row.Title, &row.CreatedAt, &row.UpdatedAt, &row.Meta)
	if err != nil {
		return nil, err
	}
	messages, err := s.conversationMessages(ctx, id)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"id":         row.ID,
		"session_id": row.SessionID,
		"user_id":    row.UserID,
		"title":      row.Title,
		"created_at": row.CreatedAt,
		"updated_at": row.UpdatedAt,
		"meta":       row.Meta,
		"messages":   messages,
	}, nil
}

func (s *Server) conversationMessages(ctx context.Context, conversationID int64) ([]map[string]any, error) {
	rows, err := s.db.Query(ctx, `
SELECT id, conversation_id, role, content, turn_id, created_at
FROM messages
WHERE conversation_id = $1
ORDER BY created_at, id`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id int64
		var convID int64
		var role string
		var content string
		var turnID *int64
		var createdAt time.Time
		if err := rows.Scan(&id, &convID, &role, &content, &turnID, &createdAt); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"id":              id,
			"conversation_id": convID,
			"role":            role,
			"content":         content,
			"turn_id":         turnID,
			"created_at":      createdAt,
		})
	}
	return out, rows.Err()
}

func (s *Server) listConversations(ctx context.Context, sessionID string, limit int, offset int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.Query(ctx, `
SELECT c.id, c.session_id, c.user_id, c.title, c.created_at, c.updated_at, c.meta,
       coalesce(m.content, '') AS last_message
FROM conversations c
LEFT JOIN LATERAL (
    SELECT content
    FROM messages
    WHERE conversation_id = c.id
    ORDER BY created_at DESC, id DESC
    LIMIT 1
) m ON true
WHERE ($1 = '' OR c.session_id = $1)
ORDER BY c.updated_at DESC, c.id DESC
LIMIT $2 OFFSET $3`, strings.TrimSpace(sessionID), limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id int64
		var rowSessionID *string
		var userID *string
		var title *string
		var createdAt time.Time
		var updatedAt time.Time
		var meta json.RawMessage
		var lastMessage string
		if err := rows.Scan(&id, &rowSessionID, &userID, &title, &createdAt, &updatedAt, &meta, &lastMessage); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"id":           id,
			"session_id":   rowSessionID,
			"user_id":      userID,
			"title":        title,
			"created_at":   createdAt,
			"updated_at":   updatedAt,
			"meta":         meta,
			"last_message": lastMessage,
		})
	}
	return out, rows.Err()
}

func (s *Server) updateConversationTitle(ctx context.Context, id int64, title string) error {
	tag, err := s.db.Exec(ctx, `
UPDATE conversations
SET title = NULLIF($2, ''), updated_at = now()
WHERE id = $1`, id, strings.TrimSpace(title))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Server) deleteConversation(ctx context.Context, id int64) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM conversations WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Server) conversationTurns(ctx context.Context, conversationID int64) ([]map[string]any, error) {
	rows, err := s.db.Query(ctx, `
SELECT id, conversation_id, trace_id, model, status, started_at, finished_at, latency_ms,
       tool_call_count, prompt_tokens, completion_tokens, total_tokens, error_code, error_message
FROM agent_turns
WHERE conversation_id = $1
ORDER BY started_at DESC, id DESC`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		row, err := scanTurnSummary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

type turnScanner interface {
	Scan(dest ...any) error
}

func scanTurnSummary(scanner turnScanner) (map[string]any, error) {
	var id int64
	var conversationID int64
	var traceID string
	var model string
	var status string
	var startedAt time.Time
	var finishedAt *time.Time
	var latencyMS *int
	var toolCallCount int
	var promptTokens *int
	var completionTokens *int
	var totalTokens *int
	var errorCode *string
	var errorMessage *string
	if err := scanner.Scan(&id, &conversationID, &traceID, &model, &status, &startedAt, &finishedAt, &latencyMS, &toolCallCount, &promptTokens, &completionTokens, &totalTokens, &errorCode, &errorMessage); err != nil {
		return nil, err
	}
	return map[string]any{
		"id":                id,
		"conversation_id":   conversationID,
		"trace_id":          traceID,
		"model":             model,
		"status":            status,
		"started_at":        startedAt,
		"finished_at":       finishedAt,
		"latency_ms":        latencyMS,
		"tool_call_count":   toolCallCount,
		"prompt_tokens":     promptTokens,
		"completion_tokens": completionTokens,
		"total_tokens":      totalTokens,
		"error_code":        errorCode,
		"error_message":     errorMessage,
	}, nil
}

func (s *Server) getTurn(ctx context.Context, traceID string) (map[string]any, error) {
	row := s.db.QueryRow(ctx, `
SELECT id, conversation_id, trace_id, model, status, started_at, finished_at, latency_ms,
       tool_call_count, prompt_tokens, completion_tokens, total_tokens, error_code, error_message
FROM agent_turns
WHERE trace_id = $1`, traceID)
	summary, err := scanTurnSummary(row)
	if err != nil {
		return nil, err
	}
	turnID, _ := summary["id"].(int64)
	calls, err := s.turnToolCalls(ctx, turnID)
	if err != nil {
		return nil, err
	}
	summary["tool_calls"] = calls
	return summary, nil
}

func (s *Server) turnToolCalls(ctx context.Context, turnID int64) ([]map[string]any, error) {
	rows, err := s.db.Query(ctx, `
SELECT id, seq, tool_call_id, tool_name, arguments, result, error, latency_ms, created_at
FROM agent_tool_calls
WHERE turn_id = $1
ORDER BY seq, id`, turnID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id int64
		var seq int
		var toolCallID *string
		var name string
		var args json.RawMessage
		var result json.RawMessage
		var errorText *string
		var latencyMS *int
		var createdAt time.Time
		if err := rows.Scan(&id, &seq, &toolCallID, &name, &args, &result, &errorText, &latencyMS, &createdAt); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"id":           id,
			"seq":          seq,
			"tool_call_id": toolCallID,
			"name":         name,
			"args":         args,
			"result":       result,
			"error":        errorText,
			"latency_ms":   latencyMS,
			"created_at":   createdAt,
		})
	}
	return out, rows.Err()
}
