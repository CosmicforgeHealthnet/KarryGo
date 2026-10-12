import 'mock_crm_store.dart';
import 'dart:convert';
import 'package:http/http.dart' as http;
import '../../core/contracts/vendor_contract.dart';
import '../../features/catalogue/state/catalogue_resource.dart';
import 'mock_catalogue_store.dart';
import 'mock_onboarding_store.dart';
import 'mock_production_operations_store.dart';

class MockCloudKitchenStore {
  MockCloudKitchenStore(
    this.business,
    this.catalogue,
    this.production, {
    required bool empty,
    required this.crm,
  }) {
    if (empty || business.branches.isEmpty) return;
    crm.bucket('corporate-customers').add({
      'corporate_ref': 'mock-corporate-profile',
      'organization_name': 'Atlas Business Group',
      'status': 'ACTIVE',
      'scope_mode': 'BUSINESS',
      'branch_refs': <String>[],
      'version': 1,
      'created_at': now,
    });
    final branch = '${business.branches.first['id']}';
    production.pools.add({
      'id': 'mock-cloud-kitchen-pool',
      'branch_id': branch,
      'capability': 'CLOUD_KITCHEN',
      'name': 'Bulk meal preparation',
      'code': 'BULK_MEALS',
      'unit_type': 'MEAL',
      'default_capacity': 200,
      'setup_buffer_minutes': 0,
      'cleanup_buffer_minutes': 0,
      'scope_type': 'BRANCH',
      'status': 'ACTIVE',
      'version': 1,
      'created_at': now,
      'updated_at': now,
    });
    packages.add({
      'id': 'mock-cloud-kitchen-package',
      'branch_id': branch,
      'name': 'Team lunch package',
      'description': 'Fresh lunch prepared for groups.',
      'status': 'ACTIVE',
      'minimum_quantity': 20,
      'maximum_quantity': 200,
      'capacity_pool_id': 'mock-cloud-kitchen-pool',
      'lead_time_minutes': 60,
      'setup_buffer_minutes': 15,
      'cleanup_buffer_minutes': 15,
      'logistics': <String, dynamic>{},
      'version': 1,
    });
    bands['mock-cloud-kitchen-package'] = [
      {
        'id': 'mock-cloud-kitchen-band',
        'min_quantity': 20,
        'max_quantity': 200,
        'unit_price_minor': 250000,
        'package_price_minor': 0,
        'lead_time_minutes': 60,
        'capacity_units_per_item': 1,
        'version': 1,
        'status': 'ACTIVE',
        'archived_at': null,
      },
    ];
    components['mock-cloud-kitchen-package'] = [
      {
        'id': 'mock-cloud-kitchen-component',
        'item_ref': 'demo-food-0',
        'default_quantity': 1,
        'variant_ref': null,
        'position': 0,
      },
    ];
    final date = DateTime.now().add(const Duration(days: 1)).toUtc();
    plans.add({
      'id': 'mock-cloud-kitchen-plan',
      'branch_id': branch,
      'corporate_profile_ref': 'mock-corporate-profile',
      'name': 'Office lunch schedule',
      'package_id': 'mock-cloud-kitchen-package',
      'recurrence_type': 'SELECTED_WEEKDAYS',
      'timezone': '${business.branches.first['timezone']}',
      'local_time': '12:00',
      'status': 'ACTIVE',
      'quantity': 25,
      'weekdays': [1, 3, 5],
      'effective_from': date.toIso8601String(),
      'effective_until': null,
      'production_minutes': 60,
      'lead_time_minutes': 60,
      'reservation_horizon_days': 30,
      'version': 1,
    });
    occurrences['mock-cloud-kitchen-plan'] = [
      for (var i = 0; i < 3; i++)
        {
          'id': 'mock-cloud-kitchen-occurrence-$i',
          'occurrence_key': 'mock-occurrence-$i',
          'occurrence_date': date.add(Duration(days: i)).toIso8601String(),
          'starts_at': date.add(Duration(days: i)).toIso8601String(),
          'ends_at': date.add(Duration(days: i, hours: 1)).toIso8601String(),
          'quantity': 25,
          'package_id': 'mock-cloud-kitchen-package',
          'state': i == 0 ? 'PLANNED' : 'CAPACITY_CONFLICT',
          'reservation_id': null,
          'production_job_id': null,
          'conflict_reason': i == 0 ? null : 'CAPACITY_UNAVAILABLE',
          'last_error': i == 0 ? '' : 'CAPACITY_UNAVAILABLE',
          'attempt_count': i == 2 ? 5 : 0,
          'replay_count': 0,
          'lease_owner': null,
          'lease_expires_at': null,
          'dead_lettered_at': i == 2 ? now : null,
          'version': 1,
        },
    ];
  }
  final MockCrmStore crm;
  final MockOnboardingStore business;
  final MockCatalogueStore catalogue;
  final MockProductionOperationsStore production;
  final packages = <JsonObject>[], plans = <JsonObject>[];
  final bands = <String, List<JsonObject>>{},
      components = <String, List<JsonObject>>{},
      occurrences = <String, List<JsonObject>>{};
  final settings = <String, JsonObject>{}, exceptions = <String, JsonObject>{};
  final replay = <String, ({String fingerprint, JsonObject result})>{};
  int sequence = 0;
  String get now => DateTime.now().toUtc().toIso8601String();
  String id(String kind) => 'mock-cloud-kitchen-$kind-${++sequence}';
  JsonObject ok(Object? data, [int status = 200]) => {
    '_data': jsonDecode(jsonEncode(data)),
    '_status': status,
  };
  JsonObject fail(String message, [int status = 422]) => {
    '_message': message,
    '_code': 'CLOUD_KITCHEN_REQUEST_REFUSED',
    '_status': status,
  };
  JsonObject? find(List<JsonObject> list, Object? id) =>
      list.where((r) => r['id'] == id).firstOrNull;
  JsonObject defaults(String branch) =>
      settings[branch] ??
      {
        'branch_id': branch,
        'bulk_order_threshold': 20,
        'minimum_lead_time_minutes': 60,
        'default_capacity_pool_ref': null,
        'reservation_hold_duration_minutes': 30,
        'corporate_reservation_horizon_days': 90,
        'version': 0,
      };
  bool validBranch(Object? branch) =>
      business.branches.any((b) => b['id'] == branch);
  bool validPool(Object? ref, Object? branch) =>
      ref == null ||
      ref == '' ||
      production.pools.any(
        (p) =>
            p['id'] == ref &&
            p['branch_id'] == branch &&
            p['capability'] == 'CLOUD_KITCHEN' &&
            p['status'] == 'ACTIVE',
      );
  int n(JsonObject b, String key, [int fallback = 0]) =>
      b[key] is int ? b[key] as int : fallback;
  JsonObject? handle(http.BaseRequest request, JsonObject body) {
    if (!request.url.path.startsWith('/api/v1/vendor/cloud-kitchen/')) {
      return null;
    }
    final path = request.url.path
        .replaceFirst('/api/v1/vendor/cloud-kitchen/', '')
        .split('/');
    if (request.method == 'GET') return read(path, request.url.queryParameters);
    final key =
        request.headers['Idempotency-Key'] ??
        request.headers['idempotency-key'];
    if (key == null || key.isEmpty) {
      return fail('A retry key is required.', 400);
    }
    final fingerprint = jsonEncode([
          request.method,
          request.url.toString(),
          body,
        ]),
        saved = replay[key];
    if (saved != null) {
      return saved.fingerprint == fingerprint
          ? saved.result
          : fail('The retry key belongs to another change.', 409);
    }
    final result = write(path, request.method, body);
    if ((result['_status'] as int) < 300) {
      replay[key] = (fingerprint: fingerprint, result: result);
    }
    return result;
  }

  JsonObject read(List<String> p, Map<String, String> q) {
    if (p.first == 'context') {
      if (!validBranch(p.last)) return fail('Kitchen not found.', 404);
      return ok({
        'branch_id': p.last,
        'currency': business.business['currency'],
      });
    }
    if (p.first == 'corporate-references') {
      final branch = q['branch_id'];
      if (!validBranch(branch)) {
        return fail('Choose an available kitchen.', 404);
      }
      final limit = int.tryParse(q['limit'] ?? '50'),
          offset = int.tryParse(q['offset'] ?? '0');
      if (limit == null ||
          limit < 1 ||
          limit > 200 ||
          offset == null ||
          offset < 0) {
        return fail('Invalid page.', 400);
      }
      final rows = corporateReferences('$branch');
      return ok({
        'items': rows.skip(offset).take(limit).toList(),
        'has_more': offset + limit < rows.length,
        'next_offset': offset + limit,
        'branch_id': branch,
      });
    }
    if (p.first == 'settings') {
      if (!validBranch(p.last)) return fail('Branch not found.', 404);
      return ok(defaults(p.last));
    }
    final list = p.first == 'bulk-packages' ? packages : plans;
    if (p.length == 1) return ok(list);
    final r = find(list, p[1]);
    if (r == null) return fail('Record not found.', 404);
    if (p.length == 2) return ok(r);
    if (p[2] == 'bands') return ok(bands[p[1]] ?? []);
    if (p[2] == 'components') return ok(components[p[1]] ?? []);
    if (p[2] == 'occurrences') return ok(occurrences[p[1]] ?? []);
    if (p[2] == 'capacity-preview') return preview(r, q);
    return fail('Resource not found.', 404);
  }

  JsonObject preview(JsonObject package, Map<String, String> q) {
    final quantity = int.tryParse(q['quantity'] ?? '') ?? 0;
    if (quantity < 1 || quantity > 1000000 || package['status'] != 'ACTIVE') {
      return fail('Choose a positive quantity and active package.');
    }
    final band = (bands['${package['id']}'] ?? [])
        .where(
          (b) =>
              b['status'] == 'ACTIVE' &&
              n(b, 'min_quantity') <= quantity &&
              (b['max_quantity'] == null || n(b, 'max_quantity') >= quantity),
        )
        .firstOrNull;
    if (band == null) {
      return fail('No active quantity band matches this quantity.');
    }
    final lead =
        band['lead_time_minutes'] ??
        package['lead_time_minutes'] ??
        defaults('${package['branch_id']}')['minimum_lead_time_minutes'];
    final earliest = DateTime.now().toUtc().add(Duration(minutes: lead as int));
    final start = q['start_at'] == null
            ? earliest
            : DateTime.tryParse(q['start_at']!),
        end = q['end_at'] == null
            ? start?.add(const Duration(hours: 1))
            : DateTime.tryParse(q['end_at']!);
    if (start == null || end == null || !end.isAfter(start)) {
      return fail('Choose a valid production window.');
    }
    final weight = quantity * n(band, 'capacity_units_per_item', 1);
    if (weight > 1000000) {
      return fail('The capacity workload exceeds its limit.');
    }
    final data = <String, dynamic>{
      'package_id': package['id'],
      'package_version': package['version'],
      'quantity': quantity,
      'matched_band': {
        for (final k in [
          'id',
          'version',
          'min_quantity',
          'max_quantity',
          'unit_price_minor',
          'package_price_minor',
          'capacity_units_per_item',
        ])
          k: band[k],
      },
      'lead_time_minutes': lead,
      'lead_time_satisfied': !start.isBefore(earliest),
      'earliest_start_at': earliest.toIso8601String(),
      'estimated_production_window': {
        'start_at': start.toIso8601String(),
        'end_at': end.toIso8601String(),
      },
    };
    if (package['capacity_pool_id'] == null ||
        package['capacity_pool_id'] == '') {
      data.addAll({
        'required_pools': [],
        'available': false,
        'blocking_reason': 'CAPACITY_POOL_NOT_CONFIGURED',
      });
      return ok(data);
    }
    final allocations = [
      {'capacity_pool_id': package['capacity_pool_id'], 'quantity': weight},
    ];
    final response = production.capacity.preview({
      'branch_id': package['branch_id'],
      'capability': 'CLOUD_KITCHEN',
      'window_start': start.toIso8601String(),
      'window_end': end.toIso8601String(),
      'allocations': allocations,
      'source_type': 'BULK_PACKAGE',
      'source_ref': package['id'],
    });
    if (response['_message'] != null) return response;
    final capacity = jsonObject(response['_data']);
    data.addAll({
      'required_pools': allocations,
      'capacity': capacity,
      'available': capacity['available'] == true && !start.isBefore(earliest),
    });
    return ok(data);
  }

  String? packageError(JsonObject b, {String? branch}) {
    if (!validBranch(branch ?? b['branch_id'])) {
      return 'Choose an existing branch.';
    }
    if ('${b['name'] ?? ''}'.trim().isEmpty || n(b, 'minimum_quantity') < 1) {
      return 'A package name and positive minimum quantity are required.';
    }
    if (b['maximum_quantity'] != null &&
        n(b, 'maximum_quantity') < n(b, 'minimum_quantity')) {
      return 'Maximum quantity must cover the minimum.';
    }
    if (!['DRAFT', 'ACTIVE'].contains(b['status'] ?? 'DRAFT') ||
        n(b, 'setup_buffer_minutes') < 0 ||
        n(b, 'setup_buffer_minutes') > 480 ||
        n(b, 'cleanup_buffer_minutes') < 0 ||
        n(b, 'cleanup_buffer_minutes') > 480) {
      return 'Review package status and preparation buffers.';
    }
    if (!validPool(b['capacity_pool_id'], branch ?? b['branch_id'])) {
      return 'Choose an active cloud-kitchen capacity pool for the package branch.';
    }
    return null;
  }

  JsonObject write(List<String> p, String method, JsonObject b) {
    if (p.first == 'settings') {
      if (!validBranch(p.last)) return fail('Branch not found.', 404);
      final current = defaults(p.last);
      if (b['version'] != current['version']) {
        return fail('Refresh kitchen settings before saving.', 409);
      }
      if (n(b, 'bulk_order_threshold') < 1 ||
          n(b, 'bulk_order_threshold') > 1000000 ||
          n(b, 'minimum_lead_time_minutes') < 0 ||
          n(b, 'minimum_lead_time_minutes') > 43200 ||
          n(b, 'reservation_hold_duration_minutes') < 1 ||
          n(b, 'reservation_hold_duration_minutes') > 1440 ||
          n(b, 'corporate_reservation_horizon_days') < 1 ||
          n(b, 'corporate_reservation_horizon_days') > 366 ||
          !validPool(b['default_capacity_pool_ref'], p.last)) {
        return fail('Review kitchen defaults and capacity scope.');
      }
      settings[p.last] = {
        ...b,
        'branch_id': p.last,
        'version': n(current, 'version') + 1,
      };
      return ok(settings[p.last]);
    }
    if (p.first == 'corporate-plans') return planWrite(p, method, b);
    if (p.length == 1) {
      final error = packageError(b);
      if (error != null) return fail(error);
      final ref = id('package');
      packages.add({
        ...b,
        'id': ref,
        'status': b['status'] ?? 'DRAFT',
        'version': 1,
        'logistics': b['logistics'] ?? {},
      });
      return ok({'id': ref}, 201);
    }
    final package = find(packages, p[1]);
    if (package == null) return fail('Package not found.', 404);
    if (package['status'] == 'ARCHIVED') {
      return fail('The package is archived.', 409);
    }
    if (p.length == 2) {
      if (b['version'] != package['version']) {
        return fail('Refresh the package version.', 409);
      }
      final error = packageError(b, branch: package['branch_id']);
      if (error != null) return fail(error);
      final branch = package['branch_id'], ver = n(package, 'version') + 1;
      package.addAll({...b, 'id': p[1], 'branch_id': branch, 'version': ver});
      return ok({'updated': true, 'version': ver});
    }
    if (p.last == 'archive' && p.length == 3) {
      if (b['expected_version'] != package['version']) {
        return fail('Refresh the package version.', 409);
      }
      package['status'] = 'ARCHIVED';
      package['version'] = n(package, 'version') + 1;
      return ok({'archived': true});
    }
    if (p[2] == 'components') {
      if (b['expected_version'] != package['version']) {
        return fail('Refresh the package version.', 409);
      }
      final list = components.putIfAbsent(p[1], () => []);
      if (method == 'DELETE') {
        final r = find(list, p.last);
        if (r == null) return fail('Component not found.', 404);
        list.remove(r);
        package['version'] = n(package, 'version') + 1;
        return ok({'removed': true, 'package_version': package['version']});
      }
      final item = catalogue.advancedRecord(
        CatalogueKind.items,
        '${b['item_ref']}',
      );
      final menu = item == null
          ? null
          : catalogue.advancedRecord(CatalogueKind.menus, '${item['menu_id']}');
      if (item == null ||
          item['status'] == 'ARCHIVED' ||
          menu?['branch_id'] != package['branch_id'] ||
          n(b, 'default_quantity') < 1) {
        return fail(
          'Choose an existing food item from the package branch and positive quantity.',
        );
      }
      final variant = b['variant_ref'] == null || b['variant_ref'] == ''
          ? null
          : catalogue.advancedRecord(
              CatalogueKind.foodVariants,
              '${b['variant_ref']}',
            );
      if (b['variant_ref'] != null &&
          b['variant_ref'] != '' &&
          (variant == null ||
              variant['item_id'] != b['item_ref'] ||
              variant['status'] != 'ACTIVE')) {
        return fail('Choose an active variant for the selected item.');
      }
      final ref = id('component');
      list.add({...b, 'id': ref}..remove('expected_version'));
      package['version'] = n(package, 'version') + 1;
      return ok({'id': ref}, 201);
    }
    if (p[2] == 'bands') return bandWrite(p, method, b, package);
    return fail('Package action not found.', 404);
  }

  JsonObject bandWrite(
    List<String> p,
    String method,
    JsonObject b,
    JsonObject package,
  ) {
    final list = bands.putIfAbsent(p[1], () => []),
        row = p.length > 3 ? find(bands[p[1]]!, p[3]) : null;
    final expected = p.length == 3
        ? b['expected_version']
        : b['expected_package_version'];
    if (expected != package['version']) {
      return fail('Refresh the package version.', 409);
    }
    if (p.length > 3 &&
        (row == null ||
            row['version'] != b['expected_band_version'] ||
            row['status'] == 'ARCHIVED')) {
      return fail('Refresh the active band version.', 409);
    }
    if (p.last == 'archive') {
      row!['status'] = 'ARCHIVED';
      row['archived_at'] = now;
      row['version'] = n(row, 'version') + 1;
      package['version'] = n(package, 'version') + 1;
      return ok({'archived': true, 'package_version': package['version']});
    }
    final min = n(b, 'min_quantity'),
        max = b['max_quantity'],
        weight = n(b, 'capacity_units_per_item', 1);
    if (min < 1 ||
        max != null && (max is! int || max < min) ||
        n(b, 'unit_price_minor') < 0 ||
        n(b, 'package_price_minor') < 0 ||
        weight < 1 ||
        weight > 1000) {
      return fail('Review quantity limits, prices and capacity units.');
    }
    if (list.any(
      (r) =>
          r != row &&
          r['status'] == 'ACTIVE' &&
          n(r, 'min_quantity') <= (max as int? ?? 2147483647) &&
          (r['max_quantity'] as int? ?? 2147483647) >= min,
    )) {
      return fail('Active quantity bands must not overlap.', 409);
    }
    if (row == null) {
      final ref = id('band');
      list.add(
        {...b, 'id': ref, 'status': 'ACTIVE', 'version': 1}
          ..remove('expected_version'),
      );
      package['version'] = n(package, 'version') + 1;
      return ok({'id': ref}, 201);
    }
    row.addAll(
      {...b, 'id': row['id'], 'version': n(row, 'version') + 1}
        ..remove('expected_package_version')
        ..remove('expected_band_version'),
    );
    package['version'] = n(package, 'version') + 1;
    return ok({
      'id': row['id'],
      'version': row['version'],
      'package_version': package['version'],
    });
  }

  List<JsonObject> corporateReferences(String branch) => [
    for (final p in crm.bucket('corporate-customers'))
      if (p['status'] == 'ACTIVE' &&
          (p['scope_mode'] == 'BUSINESS' ||
              (p['branch_refs'] as List? ?? []).contains(branch)))
        {
          'reference': p['corporate_ref'],
          'display_label': p['organization_name'],
        },
  ];
  bool validCorporate(JsonObject body, String branch) => corporateReferences(
    branch,
  ).any((p) => p['reference'] == body['corporate_profile_ref']);

  String? planError(JsonObject b, String branch) {
    final source = business.branches
        .where((r) => r['id'] == branch)
        .firstOrNull;
    if (source == null) return 'Branch not found.';
    if (b['timezone'] != null &&
        b['timezone'] != '' &&
        b['timezone'] != source['timezone']) {
      return 'Timezone must match the branch timezone.';
    }
    final package = find(packages, b['package_id']);
    if (package == null ||
        package['branch_id'] != branch ||
        package['status'] != 'ACTIVE') {
      return 'Choose an active package for the plan branch.';
    }
    if ('${b['name'] ?? ''}'.trim().isEmpty ||
        n(b, 'quantity') < 1 ||
        n(b, 'production_minutes', 60) < 1 ||
        n(b, 'production_minutes', 60) > 1440 ||
        !RegExp(r'^([01]\d|2[0-3]):[0-5]\d$').hasMatch('${b['local_time']}')) {
      return 'Review the plan name, quantity, duration and local time.';
    }
    if (![
      'DAILY',
      'WEEKLY',
      'SELECTED_WEEKDAYS',
    ].contains(b['recurrence_type'])) {
      return 'Choose a supported recurrence.';
    }
    final weekdays = b['weekdays'] as List? ?? [];
    if (b['recurrence_type'] == 'SELECTED_WEEKDAYS' && weekdays.isEmpty ||
        weekdays.any((d) => d is! int || d < 1 || d > 7) ||
        weekdays.toSet().length != weekdays.length) {
      return 'Choose unique ISO weekdays.';
    }
    final from = DateTime.tryParse('${b['effective_from']}'),
        until = b['effective_until'] == null
            ? null
            : DateTime.tryParse('${b['effective_until']}');
    if (from == null ||
        b['effective_until'] != null &&
            (until == null || until.isBefore(from))) {
      return 'Review the effective date range.';
    }
    if (b['lead_time_minutes'] != null &&
            (n(b, 'lead_time_minutes') < 0 ||
                n(b, 'lead_time_minutes') > 43200) ||
        b['reservation_horizon_days'] != null &&
            (n(b, 'reservation_horizon_days') < 1 ||
                n(b, 'reservation_horizon_days') > 366)) {
      return 'Review lead time and reservation horizon.';
    }
    return null;
  }

  JsonObject planWrite(List<String> p, String method, JsonObject b) {
    if (p.length == 1) {
      final branch = '${b['branch_id']}', error = planError(b, branch);
      if (!validCorporate(b, branch)) {
        return fail('Choose an active corporate customer for this kitchen.');
      }
      if (error != null) return fail(error);
      final ref = id('plan');
      plans.add({
        ...b,
        'id': ref,
        'timezone': business.branches
            .where((r) => r['id'] == branch)
            .first['timezone'],
        'status': 'DRAFT',
        'production_minutes': n(b, 'production_minutes', 60),
        'version': 1,
      });
      return ok({'id': ref}, 201);
    }
    final plan = find(plans, p[1]);
    if (plan == null) return fail('Plan not found.', 404);
    if (p.length == 2) {
      if (b['version'] != plan['version']) {
        return fail('Refresh the plan version.', 409);
      }
      if (!['DRAFT', 'SUSPENDED'].contains(plan['status'])) {
        return fail('Only draft or suspended plans can be edited.', 409);
      }
      if (b['corporate_profile_ref'] != plan['corporate_profile_ref'] &&
          !validCorporate(b, '${plan['branch_id']}')) {
        return fail('Choose an active corporate customer for this kitchen.');
      }
      final error = planError(b, '${plan['branch_id']}');
      if (error != null) return fail(error);
      final branch = plan['branch_id'],
          status = plan['status'],
          version = n(plan, 'version') + 1;
      plan.addAll({
        ...b,
        'id': p[1],
        'branch_id': branch,
        'status': status,
        'timezone': business.branches
            .where((r) => r['id'] == branch)
            .first['timezone'],
        'version': version,
      });
      return ok({'version': version});
    }
    if (p[2] == 'exceptions') {
      final date = DateTime.tryParse('${b['occurrence_date']}');
      if (date == null ||
          !RegExp(r'^\d{4}-\d{2}-\d{2}$').hasMatch('${b['occurrence_date']}')) {
        return fail('Use a calendar occurrence date.');
      }
      if (![
        'SKIP',
        'RESCHEDULE',
        'QUANTITY_OVERRIDE',
        'PACKAGE_OVERRIDE',
      ].contains(b['action'])) {
        return fail('Choose a supported exception.');
      }
      if (b['action'] == 'RESCHEDULE' &&
          (DateTime.tryParse('${b['replacement_date']}') == null ||
              !RegExp(
                r'^\d{4}-\d{2}-\d{2}$',
              ).hasMatch('${b['replacement_date']}'))) {
        return fail('Choose a replacement date.');
      }
      if (b['action'] == 'QUANTITY_OVERRIDE' && n(b, 'quantity_override') < 1) {
        return fail('Choose a positive replacement quantity.');
      }
      if (b['action'] == 'PACKAGE_OVERRIDE') {
        final pkg = find(packages, b['package_override']);
        if (pkg == null ||
            pkg['status'] != 'ACTIVE' ||
            pkg['branch_id'] != plan['branch_id']) {
          return fail('Choose an active package for the plan branch.');
        }
      }
      exceptions['${p[1]}|${b['occurrence_date']}'] = {...b};
      return ok(null, 201);
    }
    if (p[2] == 'occurrences') {
      final row = find(occurrences[p[1]] ?? [], p[3]);
      if (row == null) return fail('Occurrence not found.', 404);
      if (b['expected_version'] != row['version']) {
        return fail('Refresh the occurrence version.', 409);
      }
      if ('${b['reason'] ?? ''}'.length > 280) {
        return fail('The reason must be within 280 characters.');
      }
      final verb = p.last;
      if (verb == 'retry' && row['state'] != 'CAPACITY_CONFLICT' ||
          verb == 'replay' && row['dead_lettered_at'] == null ||
          verb == 'reschedule' &&
              !['PLANNED', 'CAPACITY_CONFLICT'].contains(row['state']) ||
          verb == 'cancel' && row['state'] == 'COMPLETED') {
        return fail('The occurrence does not permit this action.', 409);
      }
      if (verb == 'cancel') {
        if ('${b['reason'] ?? ''}'.trim().isEmpty) {
          return fail('A cancellation reason is required.');
        }
        final job = production.jobs
            .where((j) => j['id'] == row['production_job_id'])
            .firstOrNull;
        if (job != null &&
            ['IN_PROGRESS', 'READY', 'COMPLETED'].contains(job['status'])) {
          return fail('Production has already started.', 409);
        }
        if (job != null) {
          job['status'] = 'CANCELLED';
          job['version'] = n(job, 'version') + 1;
        }
        final reservation =
            production.capacity.reservations['${row['reservation_id']}'];
        if (reservation != null) {
          reservation['status'] = 'CANCELLED';
          reservation['version'] = n(reservation, 'version') + 1;
        }
        row['state'] = 'CANCELLED';
        row['last_error'] = b['reason'];
      } else {
        if (verb == 'reschedule') {
          final start = DateTime.tryParse('${b['starts_at']}'),
              end = DateTime.tryParse('${b['ends_at']}');
          if (start == null || end == null || !end.isAfter(start)) {
            return fail('Choose a valid production window.');
          }
          row['starts_at'] = b['starts_at'];
          row['ends_at'] = b['ends_at'];
          row['last_error'] = '';
        }
        if (verb == 'replay') {
          row['dead_lettered_at'] = null;
          row['attempt_count'] = 0;
          row['replay_count'] = n(row, 'replay_count') + 1;
        }
        row['state'] = 'PLANNED';
        row['conflict_reason'] = null;
        row['lease_owner'] = null;
        row['lease_expires_at'] = null;
        if (verb != 'reschedule') {
          row['last_error'] = b['reason'] ?? 'manual retry';
        }
      }
      row['version'] = n(row, 'version') + 1;
      return ok({
        'state': row['state'],
        'version': row['version'],
        if (verb == 'replay') 'replayed': true,
      });
    }
    if (b['expected_version'] != plan['version']) {
      return fail('Refresh the plan version.', 409);
    }
    final target = switch (p.last) {
      'activate' || 'resume' => 'ACTIVE',
      'suspend' => 'SUSPENDED',
      'archive' => 'ARCHIVED',
      _ => p.last.toUpperCase(),
    };
    final allowed = {
      'DRAFT': ['ACTIVE', 'ARCHIVED'],
      'ACTIVE': ['SUSPENDED', 'ARCHIVED'],
      'SUSPENDED': ['ACTIVE', 'ARCHIVED'],
      'ARCHIVED': <String>[],
    };
    if (allowed['${plan['status']}']?.contains(target) != true) {
      return fail('Invalid plan transition.', 409);
    }
    plan['status'] = target;
    plan['version'] = n(plan, 'version') + 1;
    return ok({'status': target});
  }
}
