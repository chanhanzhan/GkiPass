# GKiPass Frontend

This is the frontend application for GKiPass, a bidirectional tunnel control plane system.

## Overview

The frontend is built with:
- [Next.js](https://nextjs.org/) - React framework
- [TypeScript](https://www.typescriptlang.org/) - Type-safe JavaScript
- [Tailwind CSS](https://tailwindcss.com/) - Utility-first CSS framework

## Key Features

- Authentication and user management
- Node and node group management
- Tunnel configuration and monitoring
- Real-time statistics via WebSocket

## Getting Started

### Prerequisites

- Node.js 18+
- NPM or Yarn

### Installation

1. Clone the repository
2. Install dependencies:

```bash
cd web
npm install
# or
yarn
```

3. Create a `.env.local` file in the root directory:

```
# API Configuration
NEXT_PUBLIC_API_BASE_URL=http://localhost:8080/api/v1
NEXT_PUBLIC_WS_NODE_URL=ws://localhost:8080/api/v1/ws/node
NEXT_PUBLIC_WS_ADMIN_URL=ws://localhost:8080/api/v1/ws/admin

# Environment (development, production)
NODE_ENV=development
```

4. Start the development server:

```bash
npm run dev
# or
yarn dev
```

The application will be available at http://localhost:3000.

## Project Structure

```
web/
├── app/              # Next.js App Router
│   ├── (auth)/       # Authentication-related routes
│   ├── (dashboard)/  # Dashboard routes
│   ├── layout.tsx    # Root layout
│   └── page.tsx      # Home page
├── components/       # React components
│   ├── auth/         # Authentication components
│   ├── dashboard/    # Dashboard components
│   ├── nodes/        # Node-related components
│   ├── tunnels/      # Tunnel-related components
│   └── ui/           # UI components (buttons, inputs, etc.)
├── lib/              # Utility libraries
│   ├── api/          # API client
│   └── hooks/        # Custom hooks
├── public/           # Static assets
└── .env.local        # Environment variables
```

## API Integration

The frontend communicates with the backend through a structured API client. The API client is organized as follows:

- `lib/api/config.ts` - API configuration and authentication token management
- `lib/api/types.ts` - TypeScript interfaces for API requests and responses
- `lib/api/client.ts` - Base API client with methods for HTTP requests
- `lib/api/auth.ts` - Authentication service
- `lib/api/nodes.ts` - Node and node group services
- `lib/api/tunnels.ts` - Tunnel service
- `lib/api/users.ts` - User service
- `lib/api/monitoring.ts` - Monitoring service
- `lib/api/websocket.ts` - WebSocket service for real-time updates
- `lib/api/hooks.ts` - React hooks for API integration

## Custom Hooks

The frontend includes several custom hooks to simplify API usage:

- `useApiQuery` - Hook for data fetching with automatic loading and error states
- `useApiMutation` - Hook for API mutations with loading and error states
- `useAuth` - Hook for authentication state and actions

## Example Usage

### Fetching Data

```tsx
import { nodeService, useApiQuery } from '@/lib/api';

function NodeList() {
  const { data: nodes, isLoading, error, refetch } = useApiQuery(
    () => nodeService.getNodes(),
    []
  );
  
  if (isLoading) return <div>Loading...</div>;
  if (error) return <div>Error: {error}</div>;
  
  return (
    <div>
      <button onClick={refetch}>Refresh</button>
      <ul>
        {nodes?.map(node => (
          <li key={node.id}>{node.name}</li>
        ))}
      </ul>
    </div>
  );
}
```

### Authentication

```tsx
import { useAuth } from '@/components/auth/auth-context';

function LoginForm() {
  const { login, isLoading } = useAuth();
  
  const handleSubmit = async (e) => {
    e.preventDefault();
    try {
      await login(username, password);
      // Redirect on success
    } catch (error) {
      // Handle error
    }
  };
  
  return (
    <form onSubmit={handleSubmit}>
      {/* Form fields */}
      <button type="submit" disabled={isLoading}>
        {isLoading ? 'Logging in...' : 'Login'}
      </button>
    </form>
  );
}
```

## Deployment

To build the application for production:

```bash
npm run build
# or
yarn build
```

Then start the production server:

```bash
npm start
# or
yarn start
```

## Contributing

Please follow these guidelines when contributing to the project:

1. Use proper TypeScript typing
2. Follow component organization
3. Keep components focused and reusable
4. Use the provided API client for all backend communication