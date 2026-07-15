"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Image from "next/image";
import { Button } from "@/components/ui/button";
import { useFileUpload } from "@/hooks/use-file-upload";
import { useOnboardingStore } from "@/store/onboarding-store";
import { useAuthStore } from "@/store/auth-store";
import { mediaService } from "@/services/media-service";
import { profileService } from "@/services/profile-service";
import { ApiError } from "@/lib/api-client";

export default function ProfilePhotoPage() {
  const router = useRouter();
  const setPhotoFile = useOnboardingStore((s) => s.setPhotoFile);
  const token = useAuthStore((s) => s.token);
  const account = useAuthStore((s) => s.account);
  const setSession = useAuthStore((s) => s.setSession);
  const expiresAt = useAuthStore((s) => s.expiresAt);
  const { file, previewUrl, inputRef, openFilePicker, handleFileChange } =
    useFileUpload();
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const handleFinalStep = async () => {
    if (!file || loading) return;
    if (!token) {
      setError("Your session has expired. Please sign up again.");
      return;
    }
    setError(null);
    setLoading(true);
    try {
      const asset = await mediaService.uploadFile(file, "profile_photo", token);
      const { account: updatedAccount } = await profileService.updateProfile(
        { profile_photo_url: asset.url },
        token
      );
      setSession({ token, expiresAt: expiresAt ?? "", account: updatedAccount });
      setPhotoFile(file);
      router.push("/success");
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.message);
      } else {
        setError("Could not upload photo. Please try again.");
      }
    } finally {
      setLoading(false);
    }
  };

  return (
    <div>
      <h1 className="text-2xl font-bold text-ink-900">
        Upload a Photo of yourself
      </h1>
      <p className="mt-1 text-sm text-ink-500">
        A clear photo builds trust and helps customers identify you easily.
      </p>

      <div className="mt-8 flex justify-center">
        <div className="flex h-44 w-44 items-center justify-center overflow-hidden rounded-full bg-ink-100">
          {previewUrl ? (
            <Image
              src={previewUrl}
              alt="Your photo preview"
              width={176}
              height={176}
              className="h-full w-full object-cover"
            />
          ) : (
            <div
              className="h-full w-full opacity-40"
              style={{
                backgroundImage:
                  "repeating-conic-gradient(#d9d9d9 0% 25%, #f0f0f0 0% 50%)",
                backgroundSize: "16px 16px",
              }}
            />
          )}
        </div>
      </div>

      <input
        ref={inputRef}
        type="file"
        accept="image/jpeg,image/png"
        capture="user"
        onChange={handleFileChange}
        className="hidden"
      />

      {error && <p className="mt-3 text-center text-xs text-red-500">{error}</p>}

      <div className="mt-8 flex justify-center gap-3">
        <Button
          type="button"
          variant="social"
          className="w-auto rounded-full border-brand-200 bg-brand-100 px-6 py-3 text-brand-700 hover:bg-brand-200"
          onClick={openFilePicker}
        >
          Take Photo
        </Button>
        <Button
          type="button"
          className="w-auto rounded-full px-6 py-3"
          onClick={openFilePicker}
        >
          Upload Photo
        </Button>
      </div>

      <div className="mt-10">
        <Button onClick={handleFinalStep} disabled={!file || loading}>
          {loading ? "Uploading..." : "Final Step"}
        </Button>
      </div>
    </div>
  );
}