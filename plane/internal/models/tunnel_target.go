package models

// TunnelTarget 隧道目标
type TunnelTarget struct {
	Host   string `json:"host"`
	Port   int    `json:"port"`
	Weight int    `json:"weight"`
}

