import 'package:image_picker/image_picker.dart';

import '../models/media_upload_result.dart';
import 'media_file_api.dart';

class MediaUploadService {
  MediaUploadService({
    required MediaFileApi api,
    ImagePicker? picker,
  })  : _api = api,
        _picker = picker ?? ImagePicker();

  final MediaFileApi _api;
  final ImagePicker _picker;

  /// Returns null if the user cancelled the picker.
  Future<MediaUploadResult?> pickAndUpload({
    required String ownerId,
    required String purpose,
    required ImageSource source,
  }) async {
    final picked = await _picker.pickImage(
      source: source,
      maxWidth: 1024,
      maxHeight: 1024,
      imageQuality: 85,
    );
    if (picked == null) return null;

    final bytes = await picked.readAsBytes();
    final filename = picked.name.isNotEmpty ? picked.name : 'photo.jpg';

    return _api.uploadFile(
      ownerId: ownerId,
      purpose: purpose,
      filename: filename,
      bytes: bytes,
      contentType: _inferContentType(filename),
    );
  }

  Future<MediaUploadResult> uploadFromBytes({
    required String ownerId,
    required String purpose,
    required String filename,
    required List<int> bytes,
    String contentType = 'image/jpeg',
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
    if (lower.endsWith('.gif')) return 'image/gif';
    if (lower.endsWith('.webp')) return 'image/webp';
    return 'image/jpeg';
  }
}
