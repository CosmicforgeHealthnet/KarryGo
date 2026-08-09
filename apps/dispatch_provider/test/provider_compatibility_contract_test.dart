import 'package:dispatch_provider/features/auth/models/dispatch_auth_models.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  Map<String, dynamic> legacy() => <String, dynamic>{
    'provider_id': 'provider_fixture',
    'role': 'dispatch_provider',
    'token_type': 'Bearer',
    'access_token': 'sanitized-access-token',
    'refresh_token': 'sanitized-refresh-token',
    'expires_in_seconds': 900,
    'future_unknown_field': true,
  };

  test('legacy independent verify response remains parseable', () {
    final response = AuthVerifyResponse.fromJson(legacy());
    expect(response.providerId, 'provider_fixture');
    expect(response.providerMode, ProviderMode.independent);
    expect(response.organization, isNull);
  });

  test(
    'organization-managed verify response parses additive policy fields',
    () {
      final json = legacy()
        ..addAll(<String, dynamic>{
          'providerMode': 'ORGANIZATION_MANAGED',
          'organization': <String, dynamic>{
            'id': 'org_fixture',
            'displayName': 'Fixture Dispatch Co',
            'status': 'ACTIVE',
            'branch': <String, dynamic>{
              'id': 'branch_fixture',
              'name': 'Main Hub',
              'type': 'HUB',
            },
          },
          'membership': <String, dynamic>{
            'status': 'ACTIVE',
            'role': 'CONTRACTOR',
            'serviceCategory': 'DISPATCH',
          },
          'assignmentPolicy': <String, dynamic>{
            'assignmentMode': 'OFFER',
            'providerDeclineRequired': true,
          },
          'fieldVisibilityPolicy': 'ORGANIZATION',
        });

      final response = AuthVerifyResponse.fromJson(json);
      expect(response.providerMode, ProviderMode.organizationManaged);
      expect(response.organization?.id, 'org_fixture');
      expect(response.organization?.branch?.name, 'Main Hub');
      expect(response.membership?.serviceCategory, 'DISPATCH');
      expect(response.assignmentPolicy?.providerDeclineRequired, isTrue);
    },
  );
}
