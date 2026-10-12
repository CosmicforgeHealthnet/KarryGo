import 'mock_production_operations_store.dart';
import 'dart:convert';
import 'package:http/http.dart' as http;
import '../../../core/contracts/vendor_contract.dart';
import 'mock_dashboard_data.dart';

class MockSchedulingStore {
  MockSchedulingStore(this.dashboard, this.production) {
    if (dashboard.scenario == MockDashboardScenario.empty) return;
    final orders =
        (dashboard.response(
                  '/api/v1/vendor/commerce/orders',
                  {},
                )!['vendor_orders']
                as List)
            .map(jsonObject)
            .toList();
    final business = dashboard.store.business['id'];
    final now = DateTime.now().toUtc();
    schedulingQuotes.add({
      'QuoteID': 'demo-scheduling-approved-quote',
      'CustomerID': 'demo-scheduling-customer',
      'BusinessID': business,
      'BranchID': orders.first['branch_id'],
      'Currency': 'NGN',
      'QuoteVersion': 1,
      'ExpiresAt': now.add(const Duration(hours: 1)).toIso8601String(),
      'ApprovedAt': now.toIso8601String(),
      'Address': {
        'source_address_ref': 'demo-scheduling-address',
        'address': '14 Marina, Lagos',
        'latitude': 6.45,
        'longitude': 3.39,
        'snapshotted_at': now.toIso8601String(),
      },
      'Lines': [
        {
          'business_id': business,
          'branch_id': orders.first['branch_id'],
          'domain': 'FOOD',
          'item_ref': 'demo-scheduling-food',
          'quantity': 20,
          'quoted_total_minor': 8000000,
        },
      ],
    });
    for (var i = 0; i < orders.length; i++) {
      final o = orders[i];
      final at = now.add(Duration(days: i + 1));
      scheduled.add({
        'ID': 'demo-scheduled-$i',
        'BusinessID': business,
        'BranchID': o['branch_id'],
        'CustomerID': 'demo-scheduled-customer-$i',
        'QuoteID': 'demo-approved-quote-$i',
        'VendorOrderID': o['vendor_order_id'],
        'DesiredDeliveryAt': at.toIso8601String(),
        'DeliveryWindowStart': at.toIso8601String(),
        'DeliveryWindowEnd': at.add(const Duration(hours: 1)).toIso8601String(),
        'ProductionReadyBy': at
            .subtract(const Duration(minutes: 45))
            .toIso8601String(),
        'ExpectedReadyAt': at
            .subtract(const Duration(minutes: 30))
            .toIso8601String(),
        'CapacityReservationID': '',
        'CapacityStatus': 'NOT_REQUIRED',
        'FulfilmentPlanID': '',
        'TentativeVehicleClass': '',
        'SchedulingStatus': i == 3 ? 'CANCELLED' : 'ACCEPTED',
        'BulkRequirement': {},
        'SourceRef': 'scheduled:demo-$i',
        'SourceVersion': 1,
        'IdempotencyKey': 'demo-schedule-$i',
        'CreatedAt': now.toIso8601String(),
        'AcceptedAt': now.toIso8601String(),
        'UpdatedAt': now.toIso8601String(),
      });
    }
    for (var i = 0; i < 2; i++) {
      rfqs.add({
        'ID': 'demo-rfq-$i',
        'Status': i == 0 ? 'OPEN' : 'CANCELLED',
        'Servings': i == 0 ? 80 : 40,
        'RequestedAt': now.add(Duration(days: 6 + i)).toIso8601String(),
        'DeadlineAt': now.add(const Duration(days: 3)).toIso8601String(),
        'ExpiresAt': now.add(const Duration(days: 5)).toIso8601String(),
        'BudgetMinor': i == 0 ? 24000000 : null,
        'Currency': 'NGN',
        'Version': 1,
        'DestinationSnapshot': {'address': '14 Marina, Lagos'},
        'MenuRequirements': {},
        'PackagingRequirements': {},
        'DeliveryTerms': {},
        'Capacity': {'required': false},
      });
      recurring.add({
        'ID': 'demo-recurring-$i',
        'BusinessID': business,
        'BranchID': orders.first['branch_id'],
        'RequesterID': 'demo-corporate-requester-$i',
        'CorporateRef': 'demo-corporate-$i',
        'Weekdays': [1, 3, 5],
        'MealTime': '12:30',
        'Timezone': 'Africa/Lagos',
        'Status': i == 0 ? 'ACTIVE' : 'PAUSED',
        'Headcount': i == 0 ? 80 : 40,
        'AllowanceMinor': 750000,
        'DestinationSnapshot': {'address': '14 Marina, Lagos'},
        'MenuRotation': {},
        'ProductionRequirement': {},
        'TentativeVehicleClass': 'VAN',
        'AdvanceHorizonDays': 14,
        'StartsOn': now.toIso8601String(),
        'EndsOn': null,
        'Version': 1,
        'CreatedAt': now.toIso8601String(),
        'UpdatedAt': now.toIso8601String(),
      });
    }
  }
  final MockDashboardData dashboard;
  final MockProductionOperationsStore production;
  final List<JsonObject> schedulingQuotes = [];
  final Map<String, String> scheduledKeys = {};
  final List<JsonObject> scheduled = [], recurring = [];
  final Map<String, String> creationKeys = {};
  final List<JsonObject> rfqs = [], rfqQuotes = [];
  final Map<String, JsonObject> rfqCommands = {};
  JsonObject copy(JsonObject value) =>
      jsonObject(jsonDecode(jsonEncode(value)));
  JsonObject result(Object? value, {int status = 200}) => {
    '_status': status,
    '_data': value,
  };
  JsonObject error(int status, String text) => {
    '_status': status,
    '_message': text,
    '_code': status == 404 ? 'NOT_FOUND' : 'CONFLICT',
  };
  JsonObject createScheduled(http.BaseRequest request, JsonObject body) {
    final key = body['IdempotencyKey'];
    final delivery = DateTime.tryParse('${body['DesiredDeliveryAt']}');
    if (key is! String ||
        key.isEmpty ||
        delivery == null ||
        !delivery.isAfter(DateTime.now())) {
      return error(409, 'Choose a future delivery time.');
    }
    final start = DateTime.tryParse('${body['DeliveryWindowStart'] ?? ''}'),
        end = DateTime.tryParse('${body['DeliveryWindowEnd'] ?? ''}'),
        ready = DateTime.tryParse('${body['ProductionReadyBy'] ?? ''}');
    if (start != null && start.isBefore(DateTime.now()) ||
        start != null && end != null && !end.isAfter(start) ||
        ready != null && !ready.isBefore(delivery)) {
      return error(409, 'Check the delivery and preparation window.');
    }
    if (!dashboard.store.branches.any((b) => b['id'] == body['BranchID'])) {
      return error(403, 'Branch access is required.');
    }
    final prior = scheduledKeys[key];
    if (prior != null) {
      final row = scheduled.singleWhere((r) => r['ID'] == prior);
      if (row['QuoteID'] != body['QuoteID'] ||
          row['BranchID'] != body['BranchID'] ||
          row['CustomerID'] != body['CustomerID']) {
        return error(409, 'The saved schedule context changed.');
      }
      return result(copy(row), status: 201);
    }
    final quote = schedulingQuotes
        .where(
          (q) =>
              q['QuoteID'] == body['QuoteID'] &&
              q['BranchID'] == body['BranchID'] &&
              q['CustomerID'] == body['CustomerID'] &&
              DateTime.parse(q['ExpiresAt']).isAfter(DateTime.now()),
        )
        .firstOrNull;
    if (quote == null) {
      return error(409, 'A current approved checkout quote is required.');
    }
    final capacity = jsonObject(body['Capacity'] ?? {});
    String reservation = '';
    if (capacity['required'] == true) {
      final reservationBody = <String, dynamic>{
        ...capacity,
        'branch_id': body['BranchID'],
        'source_type': 'EXTERNAL_ORDER',
        'source_ref': 'scheduled:$key',
      };
      final checked = production.capacity.preview(reservationBody);
      if (checked['_message'] != null) return checked;
      if (jsonObject(checked['_data'])['available'] != true) {
        return error(409, 'Production capacity is unavailable.');
      }
      reservation = 'mock-reservation-${++production.capacity.sequence}';
      final at = DateTime.now().toUtc().toIso8601String();
      production.capacity.reservations[reservation] = {
        'id': reservation,
        ...reservationBody,
        'allocations': production.capacity.allocations(reservationBody),
        'priority': 'NORMAL',
        'status': 'CONFIRMED',
        'hold_expires_at': null,
        'created_at': at,
        'updated_at': at,
        'version': 1,
      };
      production.capacity.revision++;
    }
    final id = 'demo-scheduled-created-${scheduledKeys.length + 1}',
        now = DateTime.now().toUtc().toIso8601String();
    final row = <String, dynamic>{
      ...body,
      'ID': id,
      'BusinessID': dashboard.store.business['id'],
      'SchedulingStatus': 'ACCEPTED',
      'CapacityReservationID': reservation,
      'CapacityStatus': reservation.isEmpty ? 'NOT_REQUIRED' : 'RESERVED',
      'VendorOrderID': '',
      'FulfilmentPlanID': '',
      'BulkRequirement': {},
      'SourceRef': 'scheduled:$key',
      'SourceVersion': 1,
      'CreatedAt': now,
      'AcceptedAt': now,
      'UpdatedAt': now,
    };
    row.remove('Capacity');
    scheduled.add(copy(row));
    scheduledKeys[key] = id;
    return result(copy(row), status: 201);
  }

  JsonObject handleRFQ(
    http.BaseRequest request,
    JsonObject body,
    List<String> parts,
  ) {
    final vendor = dashboard.store.business['id'];
    if (request.method == 'GET' && parts.length == 1) {
      final limit =
          (int.tryParse(request.url.queryParameters['limit'] ?? '50') ?? 50)
              .clamp(1, 100);
      final offset =
          (int.tryParse(request.url.queryParameters['offset'] ?? '0') ?? 0)
              .clamp(0, rfqs.length);
      return result({
        'items': rfqs.skip(offset).take(limit).map(copy).toList(),
        'pagination': {
          'limit': limit,
          'offset': offset,
          'has_more': offset + limit < rfqs.length,
        },
      });
    }
    final rfq = rfqs.where((r) => r['ID'] == parts[1]).firstOrNull;
    if (rfq == null) return error(404, 'RFQ not available to this vendor.');
    if (request.method == 'GET' && parts.length == 2) return result(copy(rfq));
    if (parts.length != 3 || parts[2] != 'quotes') {
      return error(404, 'RFQ action not available.');
    }
    if (request.method == 'GET') {
      return result(
        rfqQuotes
            .where((q) => q['RFQID'] == rfq['ID'] && q['VendorID'] == vendor)
            .map(copy)
            .toList()
            .reversed
            .toList(),
      );
    }
    if (request.method != 'POST') {
      return error(404, 'RFQ action not available.');
    }
    final now = DateTime.now().toUtc();
    final expiry = DateTime.tryParse('${body['expires_at']}');
    if (!['OPEN', 'QUOTED'].contains(rfq['Status']) ||
        !DateTime.parse(rfq['DeadlineAt']).isAfter(now)) {
      return error(409, 'The request is closed or expired.');
    }
    if (body['price_minor'] is! int ||
        body['price_minor'] < 0 ||
        expiry == null ||
        !expiry.isAfter(now)) {
      return error(409, 'Check the quote amount and expiry.');
    }
    if (!dashboard.store.branches.any((b) => b['id'] == body['branch_id'])) {
      return error(403, 'Branch access is required.');
    }
    final key = '${rfq['ID']}|${body['idempotency_key']}';
    final saved = rfqCommands[key];
    if (saved != null) {
      if (saved['VendorID'] != vendor ||
          saved['BranchID'] != body['branch_id']) {
        return error(409, 'The quote key belongs to a different branch.');
      }
      return result(copy(saved), status: 201);
    }
    final previous = rfqQuotes
        .where(
          (q) =>
              q['RFQID'] == rfq['ID'] &&
              q['VendorID'] == vendor &&
              q['BranchID'] == body['branch_id'],
        )
        .toList();
    final snapshot = jsonObject(body['snapshot'] ?? {});
    final row = <String, dynamic>{
      'ID': 'demo-rfq-revision-${rfqQuotes.length + 1}',
      'QuoteID': previous.isEmpty
          ? 'demo-rfq-quote-${rfqQuotes.length + 1}'
          : previous.last['QuoteID'],
      'RFQID': rfq['ID'],
      'VendorID': vendor,
      'BranchID': body['branch_id'],
      'Revision': previous.length + 1,
      'PriceMinor': body['price_minor'],
      'Currency': body['currency'] ?? '',
      'Snapshot': snapshot,
      'Capacity': body['capacity'] ?? {},
      'VendorTerms': snapshot['vendor_terms'] ?? {},
      'PaymentPolicy': snapshot['payment_policy'] ?? {'type': 'FULL_PAYMENT'},
      'DepositPolicy': snapshot['deposit_policy'] ?? {},
      'CancellationPolicy': snapshot['cancellation_policy'] ?? {},
      'ExpiresAt': body['expires_at'],
      'Status': 'ACTIVE',
    };
    rfqQuotes.add(copy(row));
    rfqCommands[key] = copy(row);
    rfq['Status'] = 'QUOTED';
    rfq['Version']++;
    return result(copy(row), status: 201);
  }

  List<JsonObject> setupContexts({bool currentOnly = true}) => [
    for (final q in schedulingQuotes)
      if ((!currentOnly ||
              DateTime.parse(q['ExpiresAt']).isAfter(DateTime.now())) &&
          (q['Lines'] as List).every((l) => l['domain'] == 'FOOD'))
        {
          'BusinessID': q['BusinessID'],
          'BranchID': q['BranchID'],
          'RequesterID': q['CustomerID'],
          'CorporateRef': 'demo-corporate-0',
          'OrganizationName': 'Marina Business Group',
          'DestinationSnapshot': q['Address'],
          'MenuRotation': {
            'schema_version': 1,
            'mode': 'FIXED',
            'source_quote_id': q['QuoteID'],
            'quote_version': q['QuoteVersion'],
            'currency': q['Currency'],
            'lines': q['Lines'],
          },
        },
  ];
  bool sameSnapshot(Object? a, Object? b) {
    Object? normalize(Object? value) {
      if (value is Map) {
        final keys = value.keys.map((k) => '$k').toList()..sort();
        return {for (final k in keys) k: normalize(value[k])};
      }
      if (value is List) return value.map(normalize).toList();
      return value;
    }

    return jsonEncode(normalize(a)) == jsonEncode(normalize(b));
  }

  JsonObject? handle(http.BaseRequest request, JsonObject body) {
    const base = '/api/v1/vendor/commerce/section21/';
    if (!request.url.path.startsWith(base)) return null;
    final parts = request.url.path.substring(base.length).split('/');
    if (parts.first == 'recurring-setup-contexts' && request.method == 'GET') {
      final branch = request.url.queryParameters['branch_id'];
      if (branch != null &&
          !dashboard.store.branches.any((b) => b['id'] == branch)) {
        return error(403, 'Branch access is required.');
      }
      final rows = setupContexts()
          .where((r) => branch == null || r['BranchID'] == branch)
          .toList();
      final limit =
          (int.tryParse(request.url.queryParameters['limit'] ?? '50') ?? 50)
              .clamp(1, 100);
      final offset =
          (int.tryParse(request.url.queryParameters['offset'] ?? '0') ?? 0)
              .clamp(0, rows.length);
      return result({
        'items': rows.skip(offset).take(limit).map(copy).toList(),
        'pagination': {
          'limit': limit,
          'offset': offset,
          'next_offset': offset + limit,
          'has_more': offset + limit < rows.length,
        },
      });
    }
    if (parts.first == 'scheduling-quotes') {
      final branch = request.url.queryParameters['branch_id'];
      if (branch != null &&
          !dashboard.store.branches.any((b) => b['id'] == branch)) {
        return error(403, 'Branch access is required.');
      }
      final available = schedulingQuotes
          .where(
            (q) =>
                DateTime.parse(q['ExpiresAt']).isAfter(DateTime.now()) &&
                (branch == null || q['BranchID'] == branch),
          )
          .toList();
      if (request.method != 'GET') {
        return error(404, 'Quote action unavailable.');
      }
      if (parts.length == 2) {
        final q = available
            .where((q) => q['QuoteID'] == parts[1] && q['BranchID'] == branch)
            .firstOrNull;
        return q == null
            ? error(404, 'Approved quote unavailable.')
            : result(copy(q));
      }
      final limit =
              (int.tryParse(request.url.queryParameters['limit'] ?? '50') ?? 50)
                  .clamp(1, 100),
          offset =
              (int.tryParse(request.url.queryParameters['offset'] ?? '0') ?? 0)
                  .clamp(0, available.length);
      return result({
        'items': available.skip(offset).take(limit).map(copy).toList(),
        'pagination': {
          'limit': limit,
          'offset': offset,
          'has_more': offset + limit < available.length,
        },
      });
    }
    if (parts.first == 'scheduled-orders' &&
        parts.length == 1 &&
        request.method == 'POST') {
      return createScheduled(request, body);
    }
    if (parts.first == 'rfqs') return handleRFQ(request, body, parts);
    final isRecurring = parts.first == 'recurring-schedules';
    if (!isRecurring && parts.first != 'scheduled-orders') return null;
    final source = isRecurring ? recurring : scheduled;
    if (isRecurring && parts.length == 1 && request.method == 'POST') {
      final key = body['IdempotencyKey'];
      if (key is! String || key.isEmpty) {
        return error(409, 'A creation key is required.');
      }
      final existing = creationKeys[key];
      if (existing != null) {
        final prior = recurring.singleWhere((r) => r['ID'] == existing);
        if (prior['BranchID'] != body['BranchID'] ||
            prior['RequesterID'] != body['RequesterID'] ||
            prior['CorporateRef'] != body['CorporateRef']) {
          return error(409, 'The saved corporate context changed.');
        }
        return result(
          copy(recurring.singleWhere((r) => r['ID'] == existing)),
          status: 201,
        );
      }
      final menu = body['MenuRotation'];
      if (menu is Map && menu.containsKey('schema_version')) {
        bool matches(JsonObject r) =>
            r['BranchID'] == body['BranchID'] &&
            r['RequesterID'] == body['RequesterID'] &&
            r['CorporateRef'] == body['CorporateRef'] &&
            sameSnapshot(r['MenuRotation'], menu) &&
            sameSnapshot(r['DestinationSnapshot'], body['DestinationSnapshot']);
        if (!setupContexts().any(matches) && !recurring.any(matches)) {
          return error(
            409,
            'The corporate menu or delivery context is no longer available.',
          );
        }
      }
      final days = body['Weekdays'];
      final start = DateTime.tryParse('${body['StartsOn']}');
      final end = DateTime.tryParse('${body['EndsOn'] ?? ''}');
      final now = DateTime.now().toUtc();
      if (!dashboard.store.branches.any((b) => b['id'] == body['BranchID'])) {
        return error(403, 'Branch access is required.');
      }
      if (body['CorporateRef'] is! String ||
          (body['CorporateRef'] as String).isEmpty ||
          days is! List ||
          days.isEmpty ||
          days.toSet().length != days.length ||
          days.any((d) => d is! int || d < 1 || d > 7) ||
          body['Headcount'] is! int ||
          body['Headcount'] < 1 ||
          body['Headcount'] > 1000000 ||
          body['AllowanceMinor'] is! int ||
          body['AllowanceMinor'] < 0 ||
          !RegExp(
            r'^(?:[01]\d|2[0-3]):[0-5]\d$',
          ).hasMatch('${body['MealTime']}') ||
          body['AdvanceHorizonDays'] is! int ||
          body['AdvanceHorizonDays'] < 1 ||
          body['AdvanceHorizonDays'] > 90 ||
          body['Timezone'] is! String ||
          (body['Timezone'] as String).isEmpty ||
          start == null ||
          start.isBefore(DateTime.utc(now.year, now.month, now.day)) ||
          end != null && end.isBefore(start)) {
        return error(409, 'The recurring schedule is invalid.');
      }
      final id = 'demo-recurring-created-${creationKeys.length + 1}';
      final row = <String, dynamic>{
        ...body,
        'ID': id,
        'BusinessID': dashboard.store.business['id'],
        'Status': 'ACTIVE',
        'Version': 1,
        'CreatedAt': now.toIso8601String(),
        'UpdatedAt': now.toIso8601String(),
      };
      row.remove('IdempotencyKey');
      recurring.add(row);
      creationKeys[key] = id;
      return result(copy(row), status: 201);
    }

    if (request.method == 'GET' && parts.length == 1) {
      final query = request.url.queryParameters;
      final branch = query['branch_id'];
      if (branch != null &&
          !dashboard.store.branches.any((b) => b['id'] == branch)) {
        return error(403, 'Branch access is required.');
      }
      final rows = source
          .where((r) => branch == null || r['BranchID'] == branch)
          .toList();
      final limit = (int.tryParse(query['limit'] ?? '50') ?? 50).clamp(1, 100);
      final offset = (int.tryParse(query['offset'] ?? '0') ?? 0).clamp(
        0,
        rows.length,
      );
      return result({
        'items': rows.skip(offset).take(limit).map(copy).toList(),
        'pagination': {
          'limit': limit,
          'offset': offset,
          'has_more': offset + limit < rows.length,
        },
      });
    }
    if (parts.length < 2) {
      return error(404, 'Unsupported scheduling operation.');
    }
    final row = source.where((r) => r['ID'] == parts[1]).firstOrNull;
    if (row == null) return error(404, 'Schedule not found.');
    if (request.method == 'GET' && parts.length == 2) return result(copy(row));
    if (request.method == 'GET' && parts.last == 'occurrences' && isRecurring) {
      final at = DateTime.parse(row['StartsOn'] as String).toUtc();
      return result([
        for (var i = 0; i < 3; i++)
          {
            'ID': 'demo-occurrence-${row['ID']}-$i',
            'ScheduleID': row['ID'],
            'BusinessID': row['BusinessID'],
            'BranchID': row['BranchID'],
            'OccurrenceKey': '${row['ID']}:$i',
            'Timezone': row['Timezone'],
            'OccurrenceAt': at.add(Duration(days: i + 1)).toIso8601String(),
            'LocalDate': at.add(Duration(days: i + 1)).toIso8601String(),
            'Headcount': row['Headcount'],
            'AllowanceMinor': row['AllowanceMinor'],
            'Status': 'PROPOSED',
            'CapacityStatus': 'NOT_REQUIRED',
            'Version': 1,
            'ProposalID': 'demo-proposal-${row['ID']}-$i',
          },
      ]);
    }
    if (!isRecurring && request.method == 'POST' && parts.last == 'cancel') {
      if (row['SchedulingStatus'] == 'CANCELLED') {
        return error(404, 'Schedule already cancelled.');
      }
      final reservation =
          production.capacity.reservations[row['CapacityReservationID']];
      if (reservation != null &&
          ['HELD', 'CONFIRMED'].contains(reservation['status'])) {
        final released = production.capacity.handle(
          request,
          {
            'expected_version': reservation['version'],
            'reason': 'Scheduled order cancelled',
          },
          ['reservations', reservation['id'], 'release'],
        )!;
        if (released['_message'] != null) return released;
        row['CapacityStatus'] = 'RELEASED';
      }
      row['SchedulingStatus'] = 'CANCELLED';
      row['UpdatedAt'] = DateTime.now().toUtc().toIso8601String();
      return result(null, status: 204);
    }
    if (isRecurring && ['PUT', 'POST'].contains(request.method)) {
      final expected = request.method == 'PUT'
          ? int.tryParse(
              request.headers.entries
                      .where((e) => e.key.toLowerCase() == 'if-match')
                      .firstOrNull
                      ?.value ??
                  '',
            )
          : body['expected_version'];
      if (expected != row['Version'] ||
          ['CANCELLED', 'ENDED'].contains(row['Status'])) {
        return error(409, 'This schedule changed. Refresh before continuing.');
      }
      if (request.method == 'POST' && parts.last == 'status') {
        if (![
          'ACTIVE',
          'PAUSED',
          'CANCELLED',
          'ENDED',
        ].contains(body['status'])) {
          return error(409, 'Invalid schedule status.');
        }
        row['Status'] = body['status'];
      } else if (request.method == 'PUT' && parts.length == 2) {
        if (jsonObject(
              row['MenuRotation'] ?? {},
            ).containsKey('schema_version') ||
            jsonObject(
              body['MenuRotation'] ?? {},
            ).containsKey('schema_version')) {
          if (!sameSnapshot(row['MenuRotation'], body['MenuRotation']) ||
              !sameSnapshot(
                row['DestinationSnapshot'],
                body['DestinationSnapshot'],
              ) ||
              row['CorporateRef'] != body['CorporateRef'] ||
              row['RequesterID'] != body['RequesterID'] ||
              row['BranchID'] != body['BranchID']) {
            return error(
              409,
              'The fixed menu and corporate delivery context cannot be changed.',
            );
          }
        }

        final days = body['Weekdays'];
        final start = DateTime.tryParse(body['StartsOn']?.toString() ?? '');
        if (days is! List ||
            days.isEmpty ||
            days.toSet().length != days.length ||
            days.any((d) => d is! int || d < 1 || d > 7) ||
            body['Headcount'] is! int ||
            body['Headcount'] < 1 ||
            body['Headcount'] > 1000000 ||
            !RegExp(
              r'^(?:[01]\d|2[0-3]):[0-5]\d$',
            ).hasMatch(body['MealTime']?.toString() ?? '') ||
            start == null ||
            body['IdempotencyKey']?.toString().isNotEmpty != true) {
          return error(409, 'The recurring schedule is invalid.');
        }
        row.addAll({
          for (final e in body.entries)
            if (!['BusinessID', 'IdempotencyKey'].contains(e.key))
              e.key: e.value,
        });
      } else {
        return error(404, 'Unsupported scheduling operation.');
      }
      row['Version'] = (row['Version'] as int) + 1;
      row['UpdatedAt'] = DateTime.now().toUtc().toIso8601String();
      return result(null, status: 204);
    }
    return error(404, 'Unsupported scheduling operation.');
  }
}
