import 'dart:convert';
import 'dart:typed_data';
import 'package:crypto/crypto.dart';
import 'package:http/http.dart' as http;
import '../../../core/contracts/vendor_contract.dart';

class DocumentUploadClient {
  Future<void> upload(
    JsonObject grant,
    Uint8List bytes,
    String mime, {
    http.Client? transport,
  }) async {
    final uri = Uri.parse(grant['upload_url'] as String);
    final expires = DateTime.parse(grant['expires_at'] as String);
    if (!['https', 'http'].contains(uri.scheme) ||
        uri.host.isEmpty ||
        uri.userInfo.isNotEmpty ||
        !expires.isAfter(DateTime.now()) ||
        bytes.isEmpty ||
        bytes.length > (grant['max_bytes'] as num) ||
        !(grant['allowed_mime_types'] as List).contains(mime)) {
      throw const FormatException('Invalid or expired upload grant');
    }
    final owns = transport == null;
    final client = transport ?? http.Client();
    try {
      final request = http.Request('POST', uri)
        ..followRedirects = false
        ..headers.addAll({
          'Content-Type': mime,
          'X-Upload-Authorization-ID': grant['upload_session_id'] as String,
          'X-Upload-Token': grant['upload_token'] as String,
          'X-Declared-Size': '${bytes.length}',
          'X-Declared-Checksum-SHA256': sha256.convert(bytes).toString(),
        })
        ..bodyBytes = bytes;
      final response = await client
          .send(request)
          .timeout(const Duration(seconds: 60));
      final body = await response.stream.bytesToString();
      if (response.statusCode < 200 || response.statusCode >= 300) {
        throw const FormatException(
          'Upload failed. Choose the document again to start a new upload.',
        );
      }
      final envelope = jsonObject(jsonDecode(body));
      if (envelope['success'] != true) {
        throw const FormatException('Upload was not completed');
      }
      final object = jsonObject(envelope['data']);
      if (object['mediaObjectId'] != grant['media_ref'] ||
          object['uploadStatus'] != 'COMPLETED' ||
          ['UNSAFE', 'QUARANTINED'].contains(object['safetyStatus'])) {
        throw const FormatException(
          'The document is still being checked or could not be accepted.',
        );
      }
    } finally {
      if (owns) client.close();
    }
  }
}
