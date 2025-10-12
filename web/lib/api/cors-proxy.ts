"use client";

/**
 * CORS 代理助手
 * 
 * 当本地开发时，如果遇到 CORS 问题，可以使用此模块来设置代理服务
 */
import { API_BASE_URL, WS_NODE_URL, WS_ADMIN_URL } from './config';

// 检查是否需要使用代理
export function getProxiedApiUrl(endpoint: string): string {
  // 生产环境直接使用正常 URL
  if (process.env.NODE_ENV === 'production') {
    return `${API_BASE_URL}${endpoint}`;
  }

  // 开发环境 - 两种选择:
  // 1. 使用代理服务 (例如: http://localhost:3000/api/proxy)
  // 2. 直接使用 API URL 但需要确保后端配置了 CORS
  
  // 判断是否使用代理 (可以通过环境变量控制)
  const useProxy = process.env.NEXT_PUBLIC_USE_API_PROXY === 'true';
  
  if (useProxy) {
    // 使用 Next.js API 路由作为代理
    return `/api/proxy${endpoint}`;
  } else {
    // 直接访问 API (需要后端配置 CORS)
    return `${API_BASE_URL}${endpoint}`;
  }
}

// 为 WebSocket URL 获取有效的连接地址
export function getProxiedWebSocketUrl(type: 'node' | 'admin'): string {
  const wsUrl = type === 'node' ? WS_NODE_URL : WS_ADMIN_URL;
  
  // 生产环境直接使用配置的 WebSocket URL
  if (process.env.NODE_ENV === 'production') {
    return wsUrl;
  }
  
  // 开发环境可能需要修改 WebSocket URL
  // 注意: WebSocket 不能像 HTTP 请求那样使用代理路由
  // 我们可能需要在后端配置中允许前端域进行 WebSocket 连接
  
  // 这里可以根据环境变量进行定制
  const useLocalWs = process.env.NEXT_PUBLIC_USE_LOCAL_WS === 'true';
  
  if (useLocalWs) {
    // 替换为本地开发 WebSocket 地址
    // 例如，将 ws://example.com 替换为 ws://localhost:8080
    return wsUrl.replace(/^ws(s)?:\/\/[^\/]+/, 'ws$1://localhost:8080');
  }
  
  return wsUrl;
}
