import 'dart:typed_data';

import 'package:image_picker/image_picker.dart';

import '../models/media_upload_result.dart';
import 'media_file_api.dart';

/// Purposes recognised by media-file-service. Driver documents and the profile
/// photo are uploaded as images; proof/signature back the delivery confirmation.
class MediaPurpose {
  static const profilePhoto = 'profile_photo';
  static const documentFile = 'document_file';
  static const proofImage = 'proof_image';
  static const signature = 'signature';
}

class MediaUploadService {
  MediaUploadService({required MediaFileApi api}) : _api = api;

  final MediaFileApi _api;

  /// Uploads an already-picked file (the onboarding screens pick first, then
  /// upload on continue) and returns the stored public URL.
  Future<MediaUploadResult> uploadPicked({
    required String ownerId,
    required String purpose,
    required XFile file,
  }) async {
    final bytes = await file.readAsBytes();
    final filename = file.name.isNotEmpty ? file.name : 'upload.jpg';
    return _api.uploadFile(
      ownerId: ownerId,
      purpose: purpose,
      filename: filename,
      bytes: bytes,
      contentType: _inferContentType(filename),
    );
  }

  /// Uploads in-memory bytes (e.g. a rendered signature PNG) and returns the
  /// stored public URL.
  Future<MediaUploadResult> uploadBytes({
    required String ownerId,
    required String purpose,
    required String filename,
    required Uint8List bytes,
    String contentType = 'image/png',
  }) {
    return _api.uploadFile(
      ownerId: ownerId,
      purpose: purpose,
      filename: filename,
      bytes: bytes,
      contentType: contentType,
    );
  }

  void close() => _api.close();

  String _inferContentType(String filename) {
    final lower = filename.toLowerCase();
    if (lower.endsWith('.png')) return 'image/png';
    if (lower.endsWith('.webp')) return 'image/webp';
    return 'image/jpeg';
  }
}
