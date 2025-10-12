import { ApiClient } from './client';
import { ApiResponse, Plan, Subscription } from './types';

export class PlansService {
  // Get all plans
  async getPlans(enabled?: boolean): Promise<ApiResponse<Plan[]>> {
    let endpoint = '/api/plans';
    
    if (enabled !== undefined) {
      endpoint += `?enabled=${enabled}`;
    }
    
    return ApiClient.get<Plan[]>(endpoint);
  }
  
  // Get single plan
  async getPlan(id: string): Promise<ApiResponse<Plan>> {
    return ApiClient.get<Plan>(`/api/plans/${id}`);
  }
  
  // Create new plan (admin only)
  async createPlan(plan: Partial<Plan>): Promise<ApiResponse<Plan>> {
    return ApiClient.post<Plan>('/api/plans', plan);
  }
  
  // Update plan (admin only)
  async updatePlan(id: string, plan: Partial<Plan>): Promise<ApiResponse<Plan>> {
    return ApiClient.put<Plan>(`/api/plans/${id}`, plan);
  }
  
  // Delete plan (admin only)
  async deletePlan(id: string): Promise<ApiResponse<void>> {
    return ApiClient.delete<void>(`/api/plans/${id}`);
  }
  
  // Subscribe to a plan
  async subscribe(planId: string): Promise<ApiResponse<Subscription>> {
    return ApiClient.post<Subscription>(`/api/plans/${planId}/subscribe`);
  }
  
  // Get current user's subscription
  async getMySubscription(): Promise<ApiResponse<Subscription>> {
    return ApiClient.get<Subscription>('/api/user/subscription');
  }
  
  // Purchase a plan
  async purchasePlan(planId: string, paymentMethod: string): Promise<ApiResponse<any>> {
    return ApiClient.post<any>('/api/plans/purchase', {
      planId,
      paymentMethod
    });
  }
}

// Create a singleton instance
export const planService = new PlansService();

// Export the singleton instance as default
export default planService;
