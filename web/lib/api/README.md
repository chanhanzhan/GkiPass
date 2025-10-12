# GKiPass API Client

This directory contains API client code for communicating with the GKiPass backend services.

## Configuration

Create a `.env.local` file in the root of your project with the following configuration:

```
# API Configuration
NEXT_PUBLIC_API_BASE_URL=http://localhost:8080/api/v1
NEXT_PUBLIC_WS_NODE_URL=ws://localhost:8080/api/v1/ws/node
NEXT_PUBLIC_WS_ADMIN_URL=ws://localhost:8080/api/v1/ws/admin

# Environment (development, production)
NODE_ENV=development
```

## API Services

The following API services are provided:

- `authService` - Authentication (login, logout, captcha)
- `nodeService` - Node management
- `nodeGroupService` - Node group management
- `tunnelService` - Tunnel management
- `userService` - User management
- `monitoringService` - System and node monitoring

## WebSocket Services

Real-time communication with the backend is facilitated through WebSockets:

- `getAdminWebSocketService()` - Admin WebSocket connection for monitoring

## Example Usage

```typescript
import { authService } from '@/lib/api';

// Login a user
async function login(username: string, password: string) {
  try {
    const response = await authService.login({
      username,
      password
    });
    
    if (response.success) {
      console.log('Logged in successfully!');
      return response.data;
    }
  } catch (error) {
    console.error('Login failed:', error);
  }
}
```

## Error Handling

All API calls return a standardized response format:

```typescript
interface ApiResponse<T> {
  success: boolean;
  code: number;
  message?: string;
  data?: T;
  error?: string;
  request_id?: string;
  timestamp: number;
}
```

Use try/catch blocks to handle errors:

```typescript
try {
  const response = await nodeService.getNodes();
  // Process successful response
} catch (error) {
  // Handle error
  console.error('Failed to fetch nodes:', error);
}
```
