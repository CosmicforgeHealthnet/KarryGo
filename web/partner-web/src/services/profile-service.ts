import { apiClient } from "@/lib/api-client";
import type { AuthAccount } from "@/types";

export interface UpdateProfileInput {
  business_name?: string;
  business_phone?: string;
  state?: string;
  city?: string;
  business_logo_url?: string | null;
  profile_photo_url?: string | null;
}

export const profileService = {
  updateProfile(input: UpdateProfileInput, token: string): Promise<{ account: AuthAccount }> {
    return apiClient.patch<{ account: AuthAccount }>(
      "/api/v1/partner/v1/profile",
      input,
      token
    );
  },

  deleteAccount(token: string): Promise<void> {
    return apiClient.delete<void>(
      "/api/v1/partner/v1/profile/account",
      token
    );
  },
};