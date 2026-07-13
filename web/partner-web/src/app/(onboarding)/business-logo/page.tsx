"use client";

import { useRouter } from "next/navigation";
import Image from "next/image";
import { Upload } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useFileUpload } from "@/hooks/use-file-upload";
import { useOnboardingStore } from "@/store/onboarding-store";

export default function BusinessLogoPage() {
  const router = useRouter();
  const setLogoFile = useOnboardingStore((s) => s.setLogoFile);
  const { file, previewUrl, inputRef, openFilePicker, handleFileChange } =
    useFileUpload();

  const handleContinue = () => {
    setLogoFile(file);
    router.push("/profile-photo");
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

      <div className="mt-10">
        <Button onClick={handleContinue} disabled={!file}>
          Continue
        </Button>
      </div>
    </div>
  );
}