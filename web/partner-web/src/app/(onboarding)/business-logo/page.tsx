"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Image from "next/image";
import { Upload } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useFileUpload } from "@/hooks/use-file-upload";
import { useOnboardingStore } from "@/store/onboarding-store";
import { useAuthStore } from "@/store/auth-store";
import { mediaService } from "@/services/media-service";
import { profileService } from "@/services/profile-service";
import { ApiError } from "@/lib/api-client";

export default function BusinessLogoPage() {
  const router = useRouter();
  const setLogoFile = useOnboardingStore((s) => s.setLogoFile);
  const token = useAuthStore((s) => s.token);
  const account = useAuthStore((s) => s.account);
  const setSession = useAuthStore((s) => s.setSession);
  const expiresAt = useAuthStore((s) => s.expiresAt);
  const { file, previewUrl, inputRef, openFilePicker, handleFileChange } =
    useFileUpload();
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const handleContinue = async () => {
    if (!file || loading) return;
    if (!token) {
      setError("Your session has expired. Please sign up again.");
      return;
    }
    setError(null);
    setLoading(true);
    try {
      const asset = await mediaService.uploadFile(file, "document_file", token);
      const { account: updatedAccount } = await profileService.updateProfile(
        { business_logo_url: asset.url },
        token
      );
      setSession({ token, expires_at: expiresAt ?? "", account: updatedAccount });
      setLogoFile(file);
      router.push("/profile-photo");
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.message);
      } else {
        setError("Could not upload logo. Please try again.");
      }
    } finally {
      setLoading(false);
    }
  };

  return (
    <div>
      <h1 className="text-2xl font-bold text-ink-900">Business Information</h1>
      <p className="mt-1 text-sm text-ink-500">Tell us about your business.</p>

      <p className="mb-3 mt-6 text-sm font-semibold text-ink-900">
        Business Logo
      </p>

      <button
        type="button"
        onClick={openFilePicker}
        className="flex w-full flex-col items-center justify-center gap-2 rounded-xl border border-dashed border-ink-300 px-4 py-10 text-center hover:border-brand-500"
      >
        {previewUrl ? (
          <Image
            src={previewUrl}
            alt="Business logo preview"
            width={96}
            height={96}
            className="h-24 w-24 rounded-lg object-cover"
          />
        ) : (
          <>
            <Upload className="h-6 w-6 text-ink-500" />
            <span className="text-sm font-medium text-ink-900">
              Upload Image
            </span>
            <span className="text-xs text-ink-500">
              (Brand or Business Logo/Image)
            </span>
            <span className="text-xs text-ink-300">
              Supported File type: JPEG, PNG
            </span>
            <span className="text-xs text-ink-300">
              Maximum File Size: 500MB
            </span>
          </>
        )}
      </button>
      <input
        ref={inputRef}
        type="file"
        accept="image/jpeg,image/png"
        onChange={handleFileChange}
        className="hidden"
      />

      {error && <p className="mt-3 text-xs text-red-500">{error}</p>}

      <div className="mt-10">
        <Button onClick={handleContinue} disabled={!file || loading}>
          {loading ? "Uploading..." : "Continue"}
        </Button>
      </div>
    </div>
  );
}