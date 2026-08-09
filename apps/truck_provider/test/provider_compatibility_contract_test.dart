import 'package:flutter_test/flutter_test.dart';
import 'package:truck_provider/features/auth/models/provider_auth_models.dart';

void main() {
  test(
    'legacy independent login remains parseable and defaults to independent',
    () {
      final session = ProviderSession.fromJson(<String, dynamic>{
        'access_token': 'sanitized-access-token',
        'refresh_token': 'sanitized-refresh-token',
        'provider': <String, dynamic>{
          'id': 'provider_fixture',
          'phone': '+2348000000000',
          'onboarding_status': 'complete',
        },
        'future_unknown_field': <String, dynamic>{'ignored': true},
      });

      expect(session.provider.id, 'provider_fixture');
      expect(session.providerMode, ProviderMode.independent);
      expect(session.organization, isNull);
    },
  );

  test('organization-managed login extension is additive and tenant-safe', () {
    final session = ProviderSession.fromJson(<String, dynamic>{
      'access_token': 'sanitized-access-token',
      'refresh_token': 'sanitized-refresh-token',
      'provider': <String, dynamic>{
        'id': 'provider_fixture',
        'phone': '+2348000000000',
        'onboarding_status': 'complete',
      },
      'providerMode': 'ORGANIZATION_MANAGED',
      'organization': <String, dynamic>{
        'id': 'org_fixture',
        'displayName': 'Fixture Logistics',
        'status': 'ACTIVE',
        'branch': <String, dynamic>{
          'id': 'branch_fixture',
          'name': 'Main Hub',
          'type': 'HUB',
        },
      },
      'membership': <String, dynamic>{
        'status': 'ACTIVE',
        'role': 'EMPLOYEE',
        'serviceCategory': 'HAULING',
      },
      'assignmentPolicy': <String, dynamic>{
        'assignmentMode': 'OFFER',
        'providerDeclineRequired': true,
      },
      'fieldVisibilityPolicy': 'ORGANIZATION',
    });

    expect(session.providerMode, ProviderMode.organizationManaged);
    expect(session.organization?.displayName, 'Fixture Logistics');
    expect(session.assignmentPolicy?.providerDeclineRequired, isTrue);
    expect(session.fieldVisibilityPolicy, 'ORGANIZATION');
  });
}
