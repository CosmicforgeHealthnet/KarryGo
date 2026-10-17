import 'dart:convert';
import 'dart:typed_data';
import 'package:crypto/crypto.dart';
import 'package:http/http.dart' as http;
import 'package:vendor/core/contracts/vendor_contract.dart';

class MockMediaStore {
  MockMediaStore(this.business);
  final String business;
  final objects = <String, JsonObject>{};
  final contents = <String, Uint8List>{};
  final grants = <String, JsonObject>{};
  final keys = <String, JsonObject>{};
  int sequence = 0;
  http.StreamedResponse response(Object? value, {int status = 200}) =>
      http.StreamedResponse(
        Stream.value(
          utf8.encode(
            jsonEncode(
              status < 300
                  ? {'success': true, 'data': value}
                  : {
                      'success': false,
                      'error': {
                        'code': status == 404 ? 'NOT_FOUND' : 'UPLOAD_REFUSED',
                        'message': value,
                      },
                    },
            ),
          ),
        ),
        status,
        headers: {
          'content-type': 'application/json',
          'cache-control': 'no-store',
        },
      );
  bool scoped(JsonObject object, Map<String, String> target) =>
      object['target_type'] == target['target_type'] &&
      object['target_ref'] == target['target_ref'];
  Future<http.StreamedResponse?> handle(
    http.BaseRequest request,
    JsonObject body,
    bool Function(String, String, bool) eligible,
  ) async {
    final path = request.url.path;
    if (path == '/api/v1/vendor/media/uploads' && request.method == 'POST') {
      final kind = body['target_type']?.toString() ?? '',
          id = body['target_ref']?.toString() ?? '';
      final type = body['mime_type'], size = body['byte_size'];
      if (!eligible(kind, id, true)) {
        return response('Media target is unavailable.', status: 404);
      }
      if (size is! int ||
          size <= 0 ||
          size > 10 * 1024 * 1024 ||
          ![
            'image/jpeg',
            'image/png',
            'image/webp',
            if (kind == 'PASTRY_REQUEST') 'application/pdf',
          ].contains(type)) {
        return response('Unsupported file.', status: 400);
      }
      final key =
          request.headers['Idempotency-Key'] ??
          request.headers['idempotency-key'];
      if (key == null || key.isEmpty) {
        return response('Upload key required.', status: 400);
      }
      final fingerprint = jsonEncode(body);
      if (keys[key] != null) {
        if (keys[key]!['fingerprint'] != fingerprint) {
          return response('Upload key was already used.', status: 409);
        }
        return response(keys[key]!['grant']);
      }
      final ref = 'mock-asset-$business-${++sequence}',
          session = 'mock-upload-$business-$sequence',
          token = 'mock-upload-token-$business-$sequence';
      final expires = DateTime.now()
          .add(const Duration(minutes: 15))
          .toUtc()
          .toIso8601String();
      final grant = <String, dynamic>{
        'media_ref': ref,
        'upload_session_id': session,
        'upload_token': token,
        'upload_url':
            'https://mock.karryam.invalid/api/v1/media-files/vendor-uploads/$ref/content',
        'expires_at': expires,
        'max_bytes': size,
        'allowed_mime_types': [type],
      };
      objects[ref] = {
        ...body,
        'media_ref': ref,
        'upload_status': 'PENDING',
        'safety_status': 'PENDING',
        'usable': false,
      };
      grants[ref] = grant;
      keys[key] = {'fingerprint': fingerprint, 'grant': grant};
      return response(grant);
    }
    final match = RegExp(
      r'^/api/v1/vendor/media/objects/([^/]+)(/content)?$',
    ).firstMatch(path);
    if (match == null) return null;
    final object = objects[match[1]];
    final target = request.url.queryParameters;
    if (request.method != 'GET' ||
        object == null ||
        !scoped(object, target) ||
        !eligible(
          target['target_type'] ?? '',
          target['target_ref'] ?? '',
          false,
        )) {
      return response('Media file not found.', status: 404);
    }
    if (match[2] != null) {
      final bytes = contents[match[1]];
      if (object['usable'] != true || bytes == null) {
        return response('File is not ready.', status: 503);
      }
      return http.StreamedResponse(
        Stream.value(bytes),
        200,
        headers: {
          'content-type': object['mime_type'] as String,
          'cache-control': 'no-store',
        },
      );
    }
    return response(object);
  }

  http.StreamedResponse? redeem(http.BaseRequest request, Uint8List bytes) {
    final ref = request.url.path.split('/').elementAtOrNull(5);
    final grant = grants[ref], object = objects[ref];
    if (grant == null || object == null) return null;
    String? header(String key) =>
        request.headers[key] ?? request.headers[key.toLowerCase()];
    if (request.method != 'POST' ||
        header('X-Upload-Token') != grant['upload_token'] ||
        header('X-Upload-Authorization-ID') != grant['upload_session_id'] ||
        !DateTime.parse(
          grant['expires_at'] as String,
        ).isAfter(DateTime.now()) ||
        header('Content-Type') != object['mime_type'] ||
        bytes.length != object['byte_size'] ||
        header('X-Declared-Size') != '${bytes.length}' ||
        header('X-Declared-Checksum-SHA256') !=
            sha256.convert(bytes).toString()) {
      return response('Upload authorization refused.', status: 403);
    }
    if (contents.containsKey(ref)) {
      return response(
        'Upload was already completed. Check file status.',
        status: 409,
      );
    }
    final type = object['mime_type'];
    bool starts(List<int> signature) =>
        bytes.length >= signature.length &&
        List.generate(
          signature.length,
          (i) => bytes[i] == signature[i],
        ).every((v) => v);
    final valid = switch (type) {
      'image/jpeg' => starts([255, 216, 255]),
      'image/png' => starts([137, 80, 78, 71, 13, 10, 26, 10]),
      'image/webp' =>
        starts([82, 73, 70, 70]) &&
            bytes.length >= 12 &&
            utf8.decode(bytes.sublist(8, 12), allowMalformed: true) == 'WEBP',
      'application/pdf' => starts([37, 80, 68, 70]),
      _ => false,
    };
    if (!valid) {
      return response('File content does not match its type.', status: 400);
    }
    contents[ref!] = bytes;
    object.addAll({
      'upload_status': 'COMPLETED',
      'safety_status': 'SAFE',
      'usable': true,
    });
    return response({
      'mediaObjectId': ref,
      'uploadStatus': 'COMPLETED',
      'safetyStatus': 'SAFE',
    });
  }
}
