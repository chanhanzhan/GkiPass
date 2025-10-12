package service

// ServiceContainer 服务容器
type ServiceContainer struct {
	AuthService      *AuthService
	UserService      *UserService
	NodeService      *NodeService
	NodeGroupService *NodeGroupService
	TunnelService    *TunnelService
	RuleService      *RuleService
	PlanService      *PlanService
	WebSocketService *WebSocketService
}

// NewServiceContainer 创建服务容器
func NewServiceContainer() *ServiceContainer {
	return &ServiceContainer{
		AuthService:      NewAuthService(),
		NodeService:      NewNodeService(),
		RuleService:      NewRuleService(),
		WebSocketService: NewWebSocketService(),
	}
}
