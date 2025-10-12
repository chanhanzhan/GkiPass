"use client";

/**
 * Common API Types
 * 
 * This file defines TypeScript interfaces for API requests and responses.
 */

// API Response wrapper
export interface ApiResponse<T = any> {
  success: boolean;
  code: number;
  message?: string;
  data?: T;
  error?: string;
  request_id?: string;
  timestamp: number;
}

// Authentication
export interface LoginRequest {
  username: string;
  password: string;
  captcha_id?: string;
  captcha_code?: string;
}

export interface LoginResponse {
  token: string;
  user_id: string;
  username: string;
  role: string;
  expires_at: number;
}

export interface CaptchaResponse {
  captcha_id: string;
  image_data: string;
}

// User
export interface User {
  id: string;
  username: string;
  email: string;
  role: string;
  enabled: boolean;
  avatar?: string;
  created_at: string;
  updated_at: string;
  last_login_at?: string;
}

// Node
export interface Node {
  id: string;
  name: string;
  type: string; // client/server
  status: string; // online/offline/error
  ip: string;
  port: number;
  version: string;
  cert_id: string;
  api_key: string;
  group_id: string;
  user_id: string;
  last_seen: string;
  created_at: string;
  updated_at: string;
  tags: string;
  description: string;
}

// Node Group
export interface NodeGroup {
  id: string;
  name: string;
  type: string; // entry/exit
  user_id: string;
  node_count: number;
  created_at: string;
  updated_at: string;
  description: string;
}

// Tunnel
export interface Tunnel {
  id: string;
  user_id: string;
  name: string;
  protocol: string; // tcp/udp/http/https
  entry_group_id: string;
  exit_group_id: string;
  local_port: number;
  targets: string; // JSON string of targets
  enabled: boolean;
  traffic_in: number;
  traffic_out: number;
  connection_count: number;
  created_at: string;
  updated_at: string;
  description: string;
}

export interface TunnelTarget {
  host: string;
  port: number;
  weight: number;
}

// Monitoring
export interface NodeMonitoringData {
  node_id: string;
  timestamp: string;
  system_uptime: number;
  cpu_usage: number;
  memory_usage_percent: number;
  disk_usage_percent: number;
  network_interfaces: string; // JSON string
  bandwidth_in: number;
  bandwidth_out: number;
  tcp_connections: number;
  active_tunnels: number;
  traffic_in_bytes: number;
  traffic_out_bytes: number;
}

// Subscription
export interface Plan {
  id: string;
  name: string;
  max_rules: number;
  max_traffic: number;
  traffic: number;
  max_tunnels: number;
  max_bandwidth: number;
  max_connections: number;
  max_connect_ips: number;
  allowed_node_ids?: string; // JSON string
  billing_cycle: string; // monthly/yearly/permanent
  duration: number;
  price: number;
  enabled: boolean;
  created_at: string;
  updated_at: string;
  description: string;
}

export interface Subscription {
  id: string;
  user_id: string;
  plan_id: string;
  status: string; // active/expired/cancelled
  start_at: string;
  expires_at: string;
  traffic: number;
  used_traffic: number;
  max_tunnels: number;
  max_bandwidth: number;
  created_at: string;
  updated_at: string;
}

// Pagination
export interface PaginationParams {
  page?: number;
  limit?: number;
}

export interface PaginatedResponse<T> {
  data: T[];
  total: number;
  page: number;
  limit: number;
  pages: number;
}