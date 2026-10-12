import 'package:flutter_test/flutter_test.dart';
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'package:vendor/core/network/vendor_command.dart';
import 'dashboard_test.dart' show dashboardRuntime;

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  test(
    'Kitchen references use shared CRM identities and validate new plans',
    () async {
      final runtime = await dashboardRuntime();
      addTearDown(runtime.dispose);
      final api = runtime.backend.cloudKitchenApi;
      const base = '/api/v1/vendor/cloud-kitchen';
      final plans = await api.request('GET', '$base/corporate-plans');
      final plan = jsonObject((plans.data as List).first);
      final branch = '${plan['branch_id']}';
      final currency = await api.request(
        'GET',
        '$base/context/{branchId}',
        pathParameters: {'branchId': branch},
      );
      expect(currency.object['currency'], 'NGN');
      final references = await api.request(
        'GET',
        '$base/corporate-references',
        query: {'branch_id': branch, 'limit': '200', 'offset': '0'},
      );
      final choices = references.object['items'] as List;
      expect(choices.single['reference'], plan['corporate_profile_ref']);
      final corporate = await runtime.backend.crmApi.request(
        'GET',
        '/api/v1/vendor/crm/corporate-customers/{corporateId}',
        pathParameters: {'corporateId': '${plan['corporate_profile_ref']}'},
      );
      expect(corporate.object, isNotEmpty);
      final body = <String, dynamic>{
        for (final key in [
          'branch_id',
          'corporate_profile_ref',
          'name',
          'package_id',
          'quantity',
          'recurrence_type',
          'weekdays',
          'timezone',
          'local_time',
          'effective_from',
          'effective_until',
          'production_minutes',
          'lead_time_minutes',
          'reservation_horizon_days',
        ])
          key: plan[key],
      };
      final created = await api.request(
        'POST',
        '$base/corporate-plans',
        body: VendorPayload(body),
        command: VendorCommand.create(),
      );
      expect(created.object['id'], isNotEmpty);
      await expectLater(
        api.request(
          'POST',
          '$base/corporate-plans',
          body: VendorPayload({
            ...body,
            'corporate_profile_ref': 'unknown-corporate',
          }),
          command: VendorCommand.create(),
        ),
        throwsA(isA<Exception>()),
      );
      await expectLater(
        api.request(
          'GET',
          '$base/context/{branchId}',
          pathParameters: {'branchId': 'other-business-branch'},
        ),
        throwsA(isA<Exception>()),
      );
    },
  );
}
