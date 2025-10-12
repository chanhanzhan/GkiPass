import { NextRequest, NextResponse } from 'next/server';
import { API_BASE_URL } from '@/lib/api/config';

/**
 * 代理 API 请求到后端以避免 CORS 问题
 * 这个代理只在开发环境中使用
 */
export async function GET(
  request: NextRequest,
  { params }: { params: { path: string[] } }
) {
  return await proxyRequest(request, params.path, 'GET');
}

export async function POST(
  request: NextRequest,
  { params }: { params: { path: string[] } }
) {
  return await proxyRequest(request, params.path, 'POST');
}

export async function PUT(
  request: NextRequest,
  { params }: { params: { path: string[] } }
) {
  return await proxyRequest(request, params.path, 'PUT');
}

export async function DELETE(
  request: NextRequest,
  { params }: { params: { path: string[] } }
) {
  return await proxyRequest(request, params.path, 'DELETE');
}

async function proxyRequest(
  request: NextRequest,
  pathParts: string[],
  method: string
): Promise<NextResponse> {
  try {
    // 构建目标 URL
    const path = pathParts.join('/');
    const targetUrl = `${API_BASE_URL}/${path}${request.nextUrl.search}`;
    
    console.log(`Proxying ${method} request to: ${targetUrl}`);
    
    // 创建请求头
    const headers = new Headers();
    request.headers.forEach((value, key) => {
      // 过滤掉 Next.js 特定的请求头
      if (!['host', 'connection', 'content-length'].includes(key.toLowerCase())) {
        headers.append(key, value);
      }
    });
    
    // 转发请求
    const response = await fetch(targetUrl, {
      method,
      headers,
      body: ['POST', 'PUT', 'PATCH'].includes(method) ? await request.text() : undefined,
      redirect: 'follow',
      // 包含凭证（cookies 等）- 临时移除
      // credentials: 'include',
    });
    
    // 构建响应
    const responseData = await response.text();
    const responseHeaders = new Headers();
    
    response.headers.forEach((value, key) => {
      // 避免重复设置某些响应头
      if (!['content-encoding', 'content-length', 'connection'].includes(key.toLowerCase())) {
        responseHeaders.set(key, value);
      }
    });
    
    // 设置 CORS 头 - 使用具体的源而不是通配符
    const origin = request.headers.get('origin');
    if (origin) {
      responseHeaders.set('Access-Control-Allow-Origin', origin);
      responseHeaders.set('Access-Control-Allow-Credentials', 'true');
    } else {
      // 如果没有 origin 头，使用通配符
      responseHeaders.set('Access-Control-Allow-Origin', '*');
    }
    
    responseHeaders.set('Access-Control-Allow-Methods', 'GET,HEAD,PUT,PATCH,POST,DELETE');
    responseHeaders.set(
      'Access-Control-Allow-Headers', 
      'Content-Type, Authorization, X-Requested-With'
    );
    
    // 返回响应
    return new NextResponse(responseData, {
      status: response.status,
      statusText: response.statusText,
      headers: responseHeaders,
    });
  } catch (error) {
    console.error('Proxy error:', error);
    return new NextResponse(
      JSON.stringify({ error: 'Proxy request failed', message: (error as Error).message }),
      {
        status: 500,
        headers: {
          'Content-Type': 'application/json',
          'Access-Control-Allow-Origin': request.headers.get('origin') || '*',
          'Access-Control-Allow-Credentials': 'true',
        },
      }
    );
  }
}

// 预检请求处理
export async function OPTIONS(
  request: NextRequest,
  { params }: { params: { path: string[] } }
) {
  const origin = request.headers.get('origin');
  
  return new NextResponse(null, {
    status: 200,
    headers: {
      'Access-Control-Allow-Origin': origin || '*',
      'Access-Control-Allow-Credentials': 'true',
      'Access-Control-Allow-Methods': 'GET,HEAD,PUT,PATCH,POST,DELETE',
      'Access-Control-Allow-Headers': 'Content-Type, Authorization, X-Requested-With',
      'Access-Control-Max-Age': '86400', // 24 小时
    },
  });
}