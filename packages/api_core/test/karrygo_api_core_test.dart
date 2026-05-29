import 'package:flutter_test/flutter_test.dart';
import 'package:karrygo_api_core/karrygo_api_core.dart';

void main() {
  test('builds normalized API URIs', () {
    final config = ApiCoreConfig(baseUrl: 'https://api.karrygo.com/');
    expect(
      config.uri('/auth/otp').toString(),
      'https://api.karrygo.com/auth/otp',
    );
  });

  test('parses backend error envelopes', () {
    final error = ApiException.fromErrorEnvelope({
      'success': false,
      'error': {
        'code': 'validation_failed',
        'message': 'Check your details.',
        'request_id': 'req_123',
        'fields': [
          {'field': 'phone', 'message': 'Phone number is required.'},
        ],
      },
    }, statusCode: 422);

    expect(error.code, ApiErrorCode.validationFailed);
    expect(error.title, 'Check your details');
    expect(error.fields.single.field, 'phone');
  });
}
