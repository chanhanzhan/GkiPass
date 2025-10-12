"use client";

import { WS_ADMIN_URL, WS_NODE_URL, getAuthToken } from './config';
import { getProxiedWebSocketUrl } from './cors-proxy';

/**
 * WebSocket Message Types
 */
export enum MessageType {
  // Authentication and connection messages
  AUTH = 'auth',
  AUTH_RESULT = 'auth_result',
  PING = 'ping',
  PONG = 'pong',
  DISCONNECT = 'disconnect',
  
  // Rule and config messages
  RULE_SYNC = 'rule_sync',
  RULE_SYNC_ACK = 'rule_sync_ack',
  CONFIG_UPDATE = 'config_update',
  CONFIG_UPDATE_ACK = 'config_update_ack',
  
  // Status and monitoring messages
  STATUS_REPORT = 'status_report',
  METRICS_REPORT = 'metrics_report',
  LOG_REPORT = 'log_report',
  
  // Command and control messages
  COMMAND = 'command',
  COMMAND_RESULT = 'command_result',
  
  // Probe messages
  PROBE_REQUEST = 'probe_request',
  PROBE_RESULT = 'probe_result',
  
  // Error and notification messages
  ERROR = 'error',
  NOTIFICATION = 'notification',
}

/**
 * WebSocket Message
 */
export interface WebSocketMessage<T = any> {
  type: MessageType;
  id?: string;
  timestamp: number;
  data?: T;
  error?: {
    code: string;
    message: string;
  };
}

/**
 * WebSocket Connection options
 */
export interface WSConnectionOptions {
  onOpen?: () => void;
  onClose?: (event: CloseEvent) => void;
  onError?: (event: Event) => void;
  onMessage?: (message: WebSocketMessage) => void;
  autoReconnect?: boolean;
  reconnectInterval?: number;
  maxReconnectAttempts?: number;
}

/**
 * WebSocket Connection Service
 */
export class WebSocketService {
  private socket: WebSocket | null = null;
  private options: WSConnectionOptions;
  private url: string;
  private reconnectAttempts = 0;
  private reconnectTimer: NodeJS.Timeout | null = null;
  private pingInterval: NodeJS.Timeout | null = null;
  private isClosing = false;
  
  /**
   * Create a new WebSocket service instance
   */
  constructor(type: 'node' | 'admin', options: WSConnectionOptions = {}) {
    // 使用代理 WebSocket URL 来避免跨域问题
    this.url = getProxiedWebSocketUrl(type);
    this.options = {
      autoReconnect: true,
      reconnectInterval: 3000,
      maxReconnectAttempts: 10,
      ...options,
    };
    
    console.log(`WebSocket service initialized with URL: ${this.url}`);
  }
  
  /**
   * Connect to WebSocket server
   */
  connect(): Promise<void> {
    return new Promise((resolve, reject) => {
      try {
        // Close existing connection if any
        if (this.socket) {
          this.socket.close();
        }
        
        this.isClosing = false;
        
        console.log(`Connecting to WebSocket at ${this.url}`);
        
        // Create new WebSocket connection
        this.socket = new WebSocket(this.url);
        
        // Setup event handlers
        this.socket.onopen = () => {
          console.log('WebSocket connection established successfully');
          this.reconnectAttempts = 0;
          this.options.onOpen?.();
          this.startPingInterval();
          resolve();
          
          // Send authentication message
          this.authenticate();
        };
        
        this.socket.onclose = (event) => {
          console.log(`WebSocket connection closed: ${event.code} ${event.reason}`);
          this.options.onClose?.(event);
          this.stopPingInterval();
          
          // Attempt to reconnect if enabled and not deliberately closing
          if (this.options.autoReconnect && !this.isClosing) {
            this.scheduleReconnect();
          }
        };
        
        this.socket.onerror = (event) => {
          console.error('WebSocket error occurred:', event);
          this.options.onError?.(event);
          reject(event);
        };
        
        this.socket.onmessage = (event) => {
          try {
            const message = JSON.parse(event.data) as WebSocketMessage;
            this.handleMessage(message);
          } catch (error) {
            console.error('Failed to parse WebSocket message:', error);
          }
        };
      } catch (error) {
        console.error('Failed to create WebSocket connection:', error);
        reject(error);
      }
    });
  }
  
  /**
   * Disconnect from WebSocket server
   */
  disconnect(): void {
    this.isClosing = true;
    
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    
    this.stopPingInterval();
    
    if (this.socket) {
      this.socket.close();
      this.socket = null;
    }
  }
  
  /**
   * Send a message to the WebSocket server
   */
  send<T>(type: MessageType, data?: T): boolean {
    if (!this.socket || this.socket.readyState !== WebSocket.OPEN) {
      console.warn('Cannot send message: WebSocket is not connected');
      return false;
    }
    
    const message: WebSocketMessage<T> = {
      type,
      timestamp: Date.now(),
    };
    
    if (data) {
      message.data = data;
    }
    
    this.socket.send(JSON.stringify(message));
    return true;
  }
  
  /**
   * Check if the WebSocket is connected
   */
  isConnected(): boolean {
    return !!this.socket && this.socket.readyState === WebSocket.OPEN;
  }
  
  /**
   * Handle incoming WebSocket messages
   */
  private handleMessage(message: WebSocketMessage): void {
    // Handle special message types
    switch (message.type) {
      case MessageType.PING:
        this.send(MessageType.PONG);
        break;
        
      case MessageType.AUTH_RESULT:
        // Handle authentication result
        console.log('Authentication result:', message.data);
        break;
        
      default:
        // Forward message to callback if defined
        this.options.onMessage?.(message);
        break;
    }
  }
  
  /**
   * Schedule a reconnection attempt
   */
  private scheduleReconnect(): void {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
    }
    
    if (this.options.maxReconnectAttempts && 
        this.reconnectAttempts >= this.options.maxReconnectAttempts) {
      console.warn(`Maximum WebSocket reconnect attempts (${this.options.maxReconnectAttempts}) reached.`);
      return;
    }
    
    this.reconnectAttempts += 1;
    console.log(`Scheduling reconnection attempt ${this.reconnectAttempts}/${this.options.maxReconnectAttempts || '∞'} in ${this.options.reconnectInterval}ms`);
    
    this.reconnectTimer = setTimeout(() => {
      console.log(`Attempting to reconnect (attempt ${this.reconnectAttempts})...`);
      this.connect().catch(() => {
        // Failed to reconnect, try again later
        console.log('Reconnection attempt failed');
        this.scheduleReconnect();
      });
    }, this.options.reconnectInterval);
  }
  
  /**
   * Start periodic ping to keep connection alive
   */
  private startPingInterval(): void {
    this.stopPingInterval();
    
    // Send ping every 30 seconds to keep connection alive
    this.pingInterval = setInterval(() => {
      this.send(MessageType.PING);
    }, 30000);
  }
  
  /**
   * Stop ping interval
   */
  private stopPingInterval(): void {
    if (this.pingInterval) {
      clearInterval(this.pingInterval);
      this.pingInterval = null;
    }
  }
  
  /**
   * Send authentication message
   */
  private authenticate(): void {
    const token = getAuthToken();
    if (!token) {
      console.error('Cannot authenticate WebSocket: No auth token available');
      return;
    }
    
    this.send(MessageType.AUTH, { token });
  }
}

// Export singleton instances for admin and node connections
let adminWebSocketInstance: WebSocketService | null = null;

export const getAdminWebSocketService = (options?: WSConnectionOptions): WebSocketService => {
  if (!adminWebSocketInstance) {
    adminWebSocketInstance = new WebSocketService('admin', options);
  }
  return adminWebSocketInstance;
};