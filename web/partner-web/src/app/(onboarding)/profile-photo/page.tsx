"use client";

import { useRouter } from "next/navigation";
import Image from "next/image";
import { Button } from "@/components/ui/button";
import { useFileUpload } from "@/hooks/use-file-upload";
import { useOnboardingStore } from "@/store/onboarding-store";

export default function ProfilePhotoPage() {
  const router = useRouter();
  const setPhotoFile = useOnboardingStore((s) => s.setPhotoFile);
  const { file, previewUrl, inputRef, openFilePicker, handleFileChange } =
    useFileUpload();

  const handleFinalStep = () => {
    setPhotoFile(file);
    router.push("/success");
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
        <Button onClick={handleFinalStep} disabled={!file}>
          Final Step
        </Button>
      </div>
    </div>
  );
}