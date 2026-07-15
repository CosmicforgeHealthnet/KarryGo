import { ApiError } from "@/lib/api-client";

const MEDIA_BASE_URL = process.env.NEXT_PUBLIC_MEDIA_BASE_URL ?? "http://localhost:8109";

interface MediaAsset {
  id: string;
  url: string;
  purpose: string;
  content_type: string;
}

interface UploadEnvelope {
  success: boolean;
  data?: MediaAsset;
  error?: { code: string; message: string };
}

function resolveAssetUrl(url: string): string {
  if (/^https?:\/\//i.test(url)) {
    return url;
  }
  return `${MEDIA_BASE_URL}${url.startsWith("/") ? "" : "/"}${url}`;
}

export const mediaService = {
  async uploadFile(
    file: File,
    purpose: "profile_photo" | "document_file",
    token: string
  ): Promise<MediaAsset> {
    const formData = new FormData();
    formData.append("file", file);
    formData.append("purpose", purpose);

    const res = await fetch(`${MEDIA_BASE_URL}/api/v1/media/upload`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${token}`,
      },
      body: formData,
    });

    let body: UploadEnvelope;
    try {
      body = await res.json();
    } catch {
      throw new ApiError(res.status, "invalid_response", "Upload server returned an invalid response.");
    }

    if (!res.ok || !body.success || !body.data) {
      throw new ApiError(
        res.status,
        body.error?.code ?? "upload_failed",
        body.error?.message ?? "File upload failed."
      );
    }

    return { ...body.data, url: resolveAssetUrl(body.data.url) };
  },
};