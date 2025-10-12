package service

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"gkipass/plane/internal/protocol"
)

// DiagnosticsService 诊断服务
type DiagnosticsService struct {
	db          *sql.DB
	logger      *zap.Logger
	nodeService *NodeService
	wsService   *WebSocketService
}

// NewDiagnosticsService 创建诊断服务
func NewDiagnosticsService(db *sql.DB, nodeService *NodeService, wsService *WebSocketService) *DiagnosticsService {
	return &DiagnosticsService{
		db:          db,
		logger:      zap.L().Named("diagnostics-service"),
		nodeService: nodeService,
		wsService:   wsService,
	}
}

// DiagnosticResult 诊断结果
type DiagnosticResult struct {
	ID        string                   `json:"id"`
	SourceID  string                   `json:"source_id"`
	TargetID  string                   `json:"target_id"`
	Type      string                   `json:"type"`
	Status    string                   `json:"status"`
	Message   string                   `json:"message"`
	StartTime time.Time                `json:"start_time"`
	EndTime   time.Time                `json:"end_time"`
	Duration  int                      `json:"duration"` // 毫秒
	Results   []map[string]interface{} `json:"results"`
	Summary   map[string]interface{}   `json:"summary"`
}

// NodeConnectionDiagnostic 节点连接诊断
func (s *DiagnosticsService) NodeConnectionDiagnostic(sourceNodeID, targetNodeID string, diagnosticType string) (*DiagnosticResult, error) {
	// 检查源节点
	sourceNode, err := s.nodeService.GetNode(sourceNodeID)
	if err != nil {
		return nil, fmt.Errorf("源节点不存在: %w", err)
	}

	// 检查目标节点
	_, err = s.nodeService.GetNode(targetNodeID)
	if err != nil {
		return nil, fmt.Errorf("目标节点不存在: %w", err)
	}

	// 检查源节点是否在线
	if sourceNode.Status != "online" {
		return nil, fmt.Errorf("源节点不在线")
	}

	// 创建诊断结果
	result := &DiagnosticResult{
		ID:        uuid.New().String(),
		SourceID:  sourceNodeID,
		TargetID:  targetNodeID,
		Type:      diagnosticType,
		Status:    "pending",
		StartTime: time.Now(),
		Results:   make([]map[string]interface{}, 0),
		Summary:   make(map[string]interface{}),
	}

	// 保存诊断结果
	if err := s.saveDiagnosticResult(result); err != nil {
		return nil, fmt.Errorf("保存诊断结果失败: %w", err)
	}

	// 发送诊断请求
	if err := s.sendDiagnosticRequest(result); err != nil {
		// 更新诊断结果
		result.Status = "failed"
		result.Message = fmt.Sprintf("发送诊断请求失败: %s", err.Error())
		result.EndTime = time.Now()
		result.Duration = int(result.EndTime.Sub(result.StartTime).Milliseconds())

		// 保存诊断结果
		if err := s.saveDiagnosticResult(result); err != nil {
			s.logger.Error("保存诊断结果失败",
				zap.String("id", result.ID),
				zap.Error(err))
		}

		return nil, fmt.Errorf("发送诊断请求失败: %w", err)
	}

	// 返回诊断结果
	return result, nil
}

// sendDiagnosticRequest 发送诊断请求
func (s *DiagnosticsService) sendDiagnosticRequest(result *DiagnosticResult) error {
	// 创建探测请求
	probeRequest := &protocol.ProbeRequest{
		ProbeID:      result.ID,
		Type:         result.Type,
		Target:       result.TargetID,
		SourceNodeID: result.SourceID,
		TargetNodeID: result.TargetID,
		Count:        5,
		Timeout:      20,
		Protocol:     "tcp",
	}

	// 发送探测请求
	msg, err := protocol.NewMessage(protocol.MessageTypeProbeRequest, probeRequest)
	if err != nil {
		return fmt.Errorf("创建探测请求消息失败: %w", err)
	}

	// 发送消息
	if err := s.wsService.SendMessageToNode(result.SourceID, msg); err != nil {
		return fmt.Errorf("发送探测请求消息失败: %w", err)
	}

	return nil
}

// saveDiagnosticResult 保存诊断结果
func (s *DiagnosticsService) saveDiagnosticResult(result *DiagnosticResult) error {
	// 序列化结果
	resultsJSON, err := json.Marshal(result.Results)
	if err != nil {
		return fmt.Errorf("序列化结果失败: %w", err)
	}

	// 序列化摘要
	summaryJSON, err := json.Marshal(result.Summary)
	if err != nil {
		return fmt.Errorf("序列化摘要失败: %w", err)
	}

	// 检查是否存在
	var count int
	err = s.db.QueryRow(`SELECT COUNT(*) FROM diagnostics WHERE id = ?`, result.ID).Scan(&count)
	if err != nil {
		return fmt.Errorf("检查诊断结果失败: %w", err)
	}

	// 插入或更新诊断结果
	if count == 0 {
		_, err = s.db.Exec(`
			INSERT INTO diagnostics (
				id, source_id, target_id, type, status, message,
				start_time, end_time, duration, results, summary
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`,
			result.ID, result.SourceID, result.TargetID, result.Type, result.Status, result.Message,
			result.StartTime, result.EndTime, result.Duration, resultsJSON, summaryJSON,
		)
	} else {
		_, err = s.db.Exec(`
			UPDATE diagnostics SET
				status = ?, message = ?, end_time = ?, duration = ?, results = ?, summary = ?
			WHERE id = ?
		`,
			result.Status, result.Message, result.EndTime, result.Duration, resultsJSON, summaryJSON,
			result.ID,
		)
	}

	if err != nil {
		return fmt.Errorf("保存诊断结果失败: %w", err)
	}

	return nil
}

// GetDiagnosticResult 获取诊断结果
func (s *DiagnosticsService) GetDiagnosticResult(id string) (*DiagnosticResult, error) {
	var result DiagnosticResult
	var resultsJSON, summaryJSON string

	// 查询诊断结果
	err := s.db.QueryRow(`
		SELECT 
			id, source_id, target_id, type, status, message,
			start_time, end_time, duration, results, summary
		FROM diagnostics 
		WHERE id = ?
	`, id).Scan(
		&result.ID, &result.SourceID, &result.TargetID, &result.Type, &result.Status, &result.Message,
		&result.StartTime, &result.EndTime, &result.Duration, &resultsJSON, &summaryJSON,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("诊断结果不存在: %s", id)
		}
		return nil, fmt.Errorf("获取诊断结果失败: %w", err)
	}

	// 解析结果
	if resultsJSON != "" {
		if err := json.Unmarshal([]byte(resultsJSON), &result.Results); err != nil {
			return nil, fmt.Errorf("解析结果失败: %w", err)
		}
	}

	// 解析摘要
	if summaryJSON != "" {
		if err := json.Unmarshal([]byte(summaryJSON), &result.Summary); err != nil {
			return nil, fmt.Errorf("解析摘要失败: %w", err)
		}
	}

	return &result, nil
}

// ListDiagnosticResults 列出诊断结果
func (s *DiagnosticsService) ListDiagnosticResults(sourceID, targetID string, limit int) ([]*DiagnosticResult, error) {
	// 构建查询
	query := `
		SELECT 
			id, source_id, target_id, type, status, message,
			start_time, end_time, duration, results, summary
		FROM diagnostics 
		WHERE 1=1
	`

	var args []interface{}

	if sourceID != "" {
		query += " AND source_id = ?"
		args = append(args, sourceID)
	}

	if targetID != "" {
		query += " AND target_id = ?"
		args = append(args, targetID)
	}

	query += " ORDER BY start_time DESC"

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	// 查询诊断结果
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("查询诊断结果失败: %w", err)
	}
	defer rows.Close()

	// 收集诊断结果
	var results []*DiagnosticResult
	for rows.Next() {
		var result DiagnosticResult
		var resultsJSON, summaryJSON string

		if err := rows.Scan(
			&result.ID, &result.SourceID, &result.TargetID, &result.Type, &result.Status, &result.Message,
			&result.StartTime, &result.EndTime, &result.Duration, &resultsJSON, &summaryJSON,
		); err != nil {
			return nil, fmt.Errorf("扫描诊断结果失败: %w", err)
		}

		// 解析结果
		if resultsJSON != "" {
			if err := json.Unmarshal([]byte(resultsJSON), &result.Results); err != nil {
				return nil, fmt.Errorf("解析结果失败: %w", err)
			}
		}

		// 解析摘要
		if summaryJSON != "" {
			if err := json.Unmarshal([]byte(summaryJSON), &result.Summary); err != nil {
				return nil, fmt.Errorf("解析摘要失败: %w", err)
			}
		}

		results = append(results, &result)
	}

	return results, nil
}

// HandleProbeResult 处理探测结果
func (s *DiagnosticsService) HandleProbeResult(probeResult *protocol.ProbeResult) error {
	// 获取诊断结果
	result, err := s.GetDiagnosticResult(probeResult.ProbeID)
	if err != nil {
		return fmt.Errorf("获取诊断结果失败: %w", err)
	}

	// 更新诊断结果
	result.Status = "completed"
	if !probeResult.Success {
		result.Status = "failed"
	}
	result.Message = probeResult.Message
	result.EndTime = time.Now()
	result.Duration = int(result.EndTime.Sub(result.StartTime).Milliseconds())
	result.Results = probeResult.Results
	result.Summary = probeResult.Summary

	// 保存诊断结果
	if err := s.saveDiagnosticResult(result); err != nil {
		return fmt.Errorf("保存诊断结果失败: %w", err)
	}

	return nil
}

// TunnelDiagnostic 隧道诊断
func (s *DiagnosticsService) TunnelDiagnostic(tunnelID string) (*DiagnosticResult, error) {
	// TODO: 实现隧道诊断

	// 创建诊断结果
	result := &DiagnosticResult{
		ID:        uuid.New().String(),
		Type:      "tunnel",
		Status:    "pending",
		StartTime: time.Now(),
		Results:   make([]map[string]interface{}, 0),
		Summary:   make(map[string]interface{}),
	}

	// 保存诊断结果
	if err := s.saveDiagnosticResult(result); err != nil {
		return nil, fmt.Errorf("保存诊断结果失败: %w", err)
	}

	// 返回诊断结果
	return result, nil
}
