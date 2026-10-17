import 'mock_crm_store.dart';
import 'mock_production_operations_store.dart';
import 'dart:convert';
import 'package:http/http.dart' as http;
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'mock_onboarding_store.dart';

class MockPastryStore {
  MockCrmStore? crm;
  List<JsonObject> references(String branch, String kind) {
    final store = crm;
    if (store == null || !store.validBranch(branch)) return [];
    if (kind == 'CUSTOMER') {
      return [
        for (final p in store.profiles)
          if (branch == business.branches.firstOrNull?['id'] &&
              p['subject_type'] == 'CUSTOMER' &&
              p['status'] == 'ACTIVE' &&
              p['privacy_state'] == 'ACTIVE' &&
              ['NONE', 'MERGED_TARGET'].contains(p['merge_state']) &&
              store.customerRefs[p['profile_ref']] != null)
            {
              'reference': store.customerRefs[p['profile_ref']],
              'display_label': p['display_label'],
            },
      ];
    }
    return [
      for (final p in store.bucket('corporate-customers'))
        if (p['status'] == 'ACTIVE' &&
            (p['scope_mode'] == 'BUSINESS' ||
                (p['branch_refs'] as List? ?? []).contains(branch)))
          {
            'reference': p['corporate_ref'],
            'display_label': p['organization_name'],
          },
    ];
  }

  MockPastryStore(this.business, this.production, {required bool empty}) {
    if (empty || business.branches.isEmpty) return;
    final branch = business.branches.first['id'];
    production.pools.add({
      'id': 'mock-pastry-pool',
      'branch_id': branch,
      'capability': 'PASTRY_BAKERY',
      'name': 'Bakery preparation',
      'code': 'BAKERY',
      'unit_type': 'ITEM',
      'default_capacity': 100,
      'setup_buffer_minutes': 0,
      'cleanup_buffer_minutes': 0,
      'scope_type': 'BRANCH',
      'status': 'ACTIVE',
      'version': 1,
      'created_at': now,
      'updated_at': now,
    });
    products.add({
      'id': 'mock-pastry-cake',
      'branch_id': branch,
      'name': 'Celebration cake',
      'description': 'Vanilla sponge with a smooth buttercream finish.',
      'product_type': 'CAKE',
      'currency': 'NGN',
      'status': 'ACTIVE',
      'base_price_minor': 3500000,
      'customizable': true,
      'production_class': null,
      'capacity_pool_id': 'mock-pastry-pool',
      'setup_buffer_minutes': 15,
      'cleanup_buffer_minutes': 10,
      'workload_units_per_item': 1,
      'logistics': {
        'temperature_class': 'CHILLED',
        'fragile': true,
        'upright_required': true,
      },
      'version': 1,
      'position': 0,
    });
    products.add({
      'id': 'mock-pastry-croissant',
      'branch_id': branch,
      'name': 'Butter croissant',
      'description': 'Freshly baked, flaky and golden.',
      'product_type': 'OTHER',
      'currency': 'NGN',
      'status': 'ACTIVE',
      'base_price_minor': 180000,
      'customizable': false,
      'production_class': null,
      'capacity_pool_id': null,
      'setup_buffer_minutes': 10,
      'cleanup_buffer_minutes': 10,
      'workload_units_per_item': 1,
      'logistics': {},
      'version': 1,
      'position': 1,
    });
    groups.add({
      'id': 'mock-pastry-flavours',
      'name': 'Cake flavours',
      'group_type': 'FLAVOUR',
      'min_select': 1,
      'max_select': 1,
      'status': 'ACTIVE',
      'version': 1,
    });
    options['mock-pastry-flavours'] = [
      {
        'id': 'mock-pastry-vanilla',
        'name': 'Vanilla',
        'code': 'VANILLA',
        'price_delta_minor': 0,
        'workload_weight': 1,
        'position': 0,
        'capacity_class': null,
        'status': 'ACTIVE',
        'version': 1,
      },
      {
        'id': 'mock-pastry-chocolate',
        'name': 'Chocolate',
        'code': 'CHOCOLATE',
        'price_delta_minor': 300000,
        'workload_weight': 1,
        'position': 1,
        'capacity_class': null,
        'status': 'ACTIVE',
        'version': 1,
      },
    ];
    requests.add({
      'id': 'mock-pastry-request',
      'branch_id': branch,
      'customer_ref': 'mock-customer-celebration',
      'corporate_profile_ref': null,
      'title': 'Birthday celebration cake',
      'notes': 'Ready for collection on Saturday.',
      'status': 'DRAFT',
      'required_at': DateTime.now()
          .toUtc()
          .add(const Duration(days: 7))
          .toIso8601String(),
      'change_cutoff_at': null,
      'logistics': {},
      'risk_flags': [],
      'change_policy': {},
      'cancellation_policy': {},
      'version': 1,
    });
  }
  final MockOnboardingStore business;
  final MockProductionOperationsStore production;
  String get now => DateTime.now().toUtc().toIso8601String();
  final quotes = <JsonObject>[];
  final changes = <String, List<JsonObject>>{},
      milestones = <String, List<JsonObject>>{},
      histories = <String, List<JsonObject>>{};
  final intents = <String, String>{};
  final products = <JsonObject>[],
      groups = <JsonObject>[],
      requests = <JsonObject>[];
  final options = <String, List<JsonObject>>{},
      specs = <String, List<JsonObject>>{};
  final attachments = <String, Map<String, JsonObject>>{};
  final replies = <String, JsonObject>{};
  int sequence = 0;
  String next(String kind) => 'mock-pastry-$kind-${++sequence}';
  JsonObject ok(Object? data, {int status = 200}) => {
    '_data': data,
    '_status': status,
  };
  JsonObject fail(String message, {int status = 409}) => {
    '_message': message,
    '_status': status,
    '_code': status == 404 ? 'NOT_FOUND' : 'CONFLICT',
  };
  JsonObject? find(List<JsonObject> rows, String id) =>
      rows.where((r) => r['id'] == id).firstOrNull;
  JsonObject? handle(http.BaseRequest request, JsonObject body) {
    final path = request.url.path
        .split('/')
        .where((s) => s.isNotEmpty)
        .skip(4)
        .toList();
    if (path.isEmpty) return null;
    if (path.first == 'request-references' && request.method == 'GET') {
      final q = request.url.queryParameters,
          branch = q['branch_id'],
          kind = q['kind'];
      if (branch == null ||
          crm == null ||
          !crm!.validBranch(branch) ||
          !['CUSTOMER', 'CORPORATE'].contains(kind)) {
        return fail(
          'Choose a valid branch and relationship type.',
          status: 400,
        );
      }
      final all = references(branch, kind!),
          limit = int.tryParse(q['limit'] ?? '50') ?? 50,
          offset = int.tryParse(q['offset'] ?? '0') ?? 0;
      if (limit < 1 || limit > 200 || offset < 0) {
        return fail('Invalid page.', status: 400);
      }
      return ok({
        'items': all.skip(offset).take(limit).toList(),
        'has_more': all.length > offset + limit,
        'next_offset': offset + limit,
        'branch_id': branch,
        'kind': kind,
      });
    }
    final key =
        request.headers['Idempotency-Key'] ??
        request.headers['idempotency-key'];
    final cacheKey = '${request.method}|${request.url.path}|$key';
    final signature = jsonEncode(body);
    if (request.method != 'GET' &&
        key != null &&
        replies.containsKey(cacheKey)) {
      final saved = replies[cacheKey]!;
      if (saved['signature'] != signature) {
        return fail('The original request must be retried unchanged.');
      }
      return jsonObject(saved['response']);
    }
    if (request.method != 'GET' && key == null) {
      return fail('An idempotency key is required.', status: 400);
    }
    if (request.method == 'POST' &&
        path.first == 'quotes' &&
        path.last == 'approve' &&
        !business.consumePastryGrant(
          request.headers['X-Step-Up-Grant'] ??
              request.headers['x-step-up-grant'] ??
              '',
        )) {
      return fail(
        'Step-up authentication is required for quote approval.',
        status: 403,
      );
    }
    final snapshot = request.method == 'GET'
        ? null
        : jsonObject(
            jsonDecode(
              jsonEncode({
                'products': products,
                'groups': groups,
                'requests': requests,
                'options': options,
                'specs': specs,
                'quotes': quotes,
                'changes': changes,
                'milestones': milestones,
                'histories': histories,
                'intents': intents,
                'reservations': production.capacity.reservations,
                'jobs': production.jobs,
                'events': production.events,
                'revision': production.capacity.revision,
              }),
            ),
          );
    final result =
        operational(request.method, path, body, request.url.queryParameters) ??
        dispatch(request.method, path, body);
    if (result?['_message'] != null && snapshot != null) {
      void rows(List<JsonObject> target, String name) {
        target
          ..clear()
          ..addAll((snapshot[name] as List).map(jsonObject));
      }

      void lists(Map<String, List<JsonObject>> target, String name) {
        target
          ..clear()
          ..addAll({
            for (final e in jsonObject(snapshot[name]).entries)
              e.key: (e.value as List).map(jsonObject).toList(),
          });
      }

      rows(products, 'products');
      rows(groups, 'groups');
      rows(requests, 'requests');
      rows(quotes, 'quotes');
      rows(production.jobs, 'jobs');
      lists(options, 'options');
      lists(specs, 'specs');
      lists(changes, 'changes');
      lists(milestones, 'milestones');
      lists(histories, 'histories');
      lists(production.events, 'events');
      intents
        ..clear()
        ..addAll(Map<String, String>.from(snapshot['intents']));
      production.capacity.reservations
        ..clear()
        ..addAll({
          for (final e in jsonObject(snapshot['reservations']).entries)
            e.key: jsonObject(e.value),
        });
      production.capacity.revision = snapshot['revision'];
    }
    if (result != null &&
        key != null &&
        request.method != 'GET' &&
        result['_message'] == null) {
      replies[cacheKey] = {'signature': signature, 'response': result};
    }
    return result;
  }

  JsonObject? dispatch(String method, List<String> path, JsonObject body) {
    final rows = switch (path.first) {
      'products' => products,
      'option-groups' => groups,
      'custom-requests' => requests,
      _ => null,
    };
    if (path.first == 'options' && path.length >= 2) {
      for (final list in options.values) {
        final row = find(list, path[1]);
        if (row == null) continue;
        if (row['version'] != (body['version'] ?? body['expected_version'])) {
          return fail('Option changed. Refresh before editing.');
        }
        if (path.last == 'archive') {
          row['status'] = 'ARCHIVED';
          row['version']++;
          bumpAttached(path[1], option: true);
          return ok(null);
        }
        if (method == 'PATCH') {
          row.addAll({...body, 'version': row['version'] + 1});
          bumpAttached(path[1], option: true);
          return ok(null);
        }
      }
      return fail('Option not found.', status: 404);
    }
    if (rows == null) return null;
    if (path.length == 1) {
      if (method == 'GET') return ok(rows);
      if (method != 'POST') return null;
      if (path.first != 'option-groups' &&
          !business.branches.any(
            (b) =>
                b['id'] == body['branch_id'] &&
                (b['capabilities'] as List).any(
                  (c) =>
                      c['capability_type'] == 'PASTRY_BAKERY' &&
                      c['status'] == 'ACTIVE',
                ),
          )) {
        return fail('Choose an enabled pastry branch.', status: 400);
      }
      final row = <String, dynamic>{
        ...body,
        'id': next(path.first),
        'version': 1,
        'status': body['status'] ?? 'DRAFT',
      };
      if (path.first == 'products') {
        row.addAll({
          'currency': body['currency'] ?? 'NGN',
          'logistics': body['logistics'] ?? {},
          'workload_units_per_item': body['workload_units_per_item'] ?? 1,
        });
      }
      if (path.first == 'option-groups') {
        if ((body['max_select'] ?? 0) < (body['min_select'] ?? 0)) {
          return fail(
            'Maximum selections must cover the minimum.',
            status: 400,
          );
        }
        row['status'] = 'ACTIVE';
      }
      if (path.first == 'custom-requests') {
        for (final entry in {
          'customer_ref': 'CUSTOMER',
          'corporate_profile_ref': 'CORPORATE',
        }.entries) {
          final value = body[entry.key];
          if (value != null &&
              value != '' &&
              !references(
                '${body['branch_id']}',
                entry.value,
              ).any((r) => r['reference'] == value)) {
            return fail(
              'The selected relationship is unavailable for this branch.',
              status: 400,
            );
          }
        }
        const milestones = [
          'DESIGN_APPROVED',
          'BASE_PREPARED',
          'DECORATION',
          'CHILLING',
          'PACKED',
          'READY',
        ];
        for (final policy in ['change_policy', 'cancellation_policy']) {
          final value = body[policy];
          if (value != null && value is! Map) {
            return fail('Check the request policy.', status: 400);
          }
          final fields = policy == 'change_policy'
              ? ['minutes_before_required_at', 'disallowed_after_milestone']
              : [
                  'review_minutes_before_required_at',
                  'restrict_minutes_before_required_at',
                  'review_after_milestone',
                  'restrict_after_milestone',
                ];
          for (final entry in jsonObject(value ?? {}).entries) {
            if (!fields.contains(entry.key)) {
              return fail('Unsupported policy field.', status: 400);
            }
            if (entry.key.contains('minutes')) {
              if (entry.value is! int ||
                  entry.value < 0 ||
                  entry.value > 129600) {
                return fail('Check the policy window.', status: 400);
              }
            } else if (entry.value != '' && !milestones.contains(entry.value)) {
              return fail('Choose a supported milestone.', status: 400);
            }
          }
        }
        const risks = [
          'LATE_START',
          'CAPACITY_AT_RISK',
          'CUSTOMER_CHANGE_PENDING',
          'MATERIAL_DEPENDENCY',
          'QUALITY_REVIEW_REQUIRED',
        ];
        final flags = body['risk_flags'] ?? [];
        if (flags is! List ||
            flags.length > 12 ||
            flags.any((v) => !risks.contains(v))) {
          return fail('Choose supported risk flags.', status: 400);
        }
        row['required_at'] = row.remove('event_at');
        row['logistics'] ??= {};
        row['risk_flags'] ??= [];
        row['change_policy'] ??= {};
        row['cancellation_policy'] ??= {};
      }
      rows.add(row);
      return ok({
        'id': row['id'],
        if (path.first == 'custom-requests') 'version': 1,
      }, status: 201);
    }
    final row = find(rows, path[1]);
    if (row == null) return fail('Pastry record not found.', status: 404);
    if (path.length == 2) {
      if (method == 'GET') return ok(row);
      if (method == 'PATCH') {
        if (row['version'] != (body['version'] ?? body['expected_version'])) {
          return fail('This record changed. Refresh before editing.');
        }
        if (path.first == 'custom-requests') {
          if (!['DRAFT', 'SUBMITTED'].contains(row['status'])) {
            return fail('This request can no longer be edited.');
          }
          row.addAll({
            'title': body['title'],
            'notes': body['notes'] ?? '',
            'required_at': body['event_at'],
            'version': row['version'] + 1,
          });
          return ok({'id': row['id']});
        }
        if (path.first == 'products') {
          final fields = [
            'name',
            'description',
            'base_price_minor',
            'customizable',
            'production_class',
            'capacity_pool_id',
            'setup_buffer_minutes',
            'cleanup_buffer_minutes',
            'workload_units_per_item',
            'logistics',
            'position',
          ];
          for (final f in fields) {
            row[f] = body[f];
          }
        } else {
          if ((body['max_select'] ?? 0) < (body['min_select'] ?? 0)) {
            return fail(
              'Maximum selections must cover the minimum.',
              status: 400,
            );
          }
          for (final f in ['name', 'min_select', 'max_select']) {
            row[f] = body[f];
          }
        }
        row['version']++;
        if (path.first == 'option-groups') {
          bumpAttached(path[1]);
          return ok(null);
        }
        return ok({'version': row['version']});
      }
    }
    if (path[2] == 'archive') {
      if (row['version'] != body['expected_version']) {
        return fail('This record changed. Refresh before archiving.');
      }
      row['status'] = 'ARCHIVED';
      row['version']++;
      if (path.first == 'option-groups') bumpAttached(path[1]);
      return ok(null);
    }
    if (path.first == 'option-groups' && path[2] == 'options') {
      final list = options.putIfAbsent(path[1], () => []);
      if (method == 'GET') return ok(list);
      if (method == 'POST') {
        if ((body['price_delta_minor'] ?? 0) < 0 ||
            (body['workload_weight'] ?? 0) <= 0 ||
            (body['position'] ?? 0) > 999) {
          return fail(
            'Check the option price, workload and position.',
            status: 400,
          );
        }
        final id = next('option');
        list.add({...body, 'id': id, 'status': 'ACTIVE', 'version': 1});
        bumpAttached(path[1]);
        return ok({'id': id}, status: 201);
      }
    }
    if (path.first == 'products' &&
        path[2] == 'option-groups' &&
        method == 'GET') {
      return ok([
        for (final a in (attachments[path[1]] ?? {}).values)
          if (find(groups, '${a['group_id']}') case final g?)
            {
              for (final e in g.entries)
                if (e.key != 'id') e.key: e.value,
              'group_id': g['id'],
              'position': a['position'],
              'required': a['required'],
            },
      ]);
    }
    if (path.first == 'products' &&
        path.length == 4 &&
        path[2] == 'option-groups' &&
        method == 'DELETE') {
      if (row['version'] != body['expected_version']) {
        return fail('Product changed. Refresh before removing options.');
      }
      if (attachments[path[1]]?.remove(path[3]) == null) {
        return fail('Attachment not found.', status: 404);
      }
      row['version']++;
      return ok(null);
    }
    if (path.first == 'products' &&
        path[2] == 'option-groups' &&
        method == 'POST') {
      if (row['version'] != body['expected_version']) {
        return fail('Product changed. Refresh before attaching options.');
      }
      if (find(groups, '${body['group_id']}') == null) {
        return fail('Option group not found.', status: 404);
      }
      attachments.putIfAbsent(path[1], () => {})['${body['group_id']}'] = {
        ...body,
      };
      row['version']++;
      return ok(null);
    }
    if (path.first != 'custom-requests') {
      return fail(
        'This pastry action is not available in this Mock workflow.',
        status: 400,
      );
    }
    final requestId = path[1];
    if (path[2] == 'specifications') {
      final list = specs.putIfAbsent(requestId, () => []);
      if (method == 'GET') return ok(list);
      if (method == 'POST') {
        if (row['version'] != body['expected_version']) {
          return fail(
            'Request changed. Refresh before saving a specification.',
          );
        }
        if ([
          'ACCEPTED',
          'ACCEPTED_PENDING_ORDER',
          'CONVERTED',
          'CANCELLED',
        ].contains(row['status'])) {
          return fail('This request is outside its change window.');
        }
        if (eligibility(row)['allowed'] != true) {
          return fail('This request is outside its change window.');
        }
        final spec = jsonObject(body['specification']);
        if (spec.keys.any(
          (k) => ![
            'occasion',
            'required_at',
            'servings',
            'theme',
            'colours',
            'inscription',
            'selected_options',
            'reference_media',
            'notes',
            'production_requirements',
            'logistics_requirements',
          ].contains(k),
        )) {
          return fail('Unsupported specification field.', status: 400);
        }
        if (spec['servings'] != null &&
            (spec['servings'] is! int ||
                spec['servings'] < 1 ||
                spec['servings'] > 10000)) {
          return fail('Enter between 1 and 10,000 servings.', status: 400);
        }
        final media = spec['reference_media'];
        if (media != null &&
            (media is! List ||
                media.length > 20 ||
                media.any(
                  (v) => v is! String || v.trim().isEmpty || v.length > 200,
                ))) {
          return fail('Use up to 20 valid Media references.', status: 400);
        }
        if (spec['notes'] != null &&
            (spec['notes'] is! String ||
                utf8.encode(spec['notes']).length > 4000)) {
          return fail('Design notes are too long.', status: 400);
        }
        final id = next('specification'), revision = list.length + 1;
        list.add({
          'id': id,
          'revision': revision,
          'specification': spec,
          'created_at': DateTime.now().toUtc().toIso8601String(),
        });
        row['version']++;
        return ok({'id': id, 'revision': revision}, status: 201);
      }
    }

    if (path[2] == 'change-eligibility') return ok(eligibility(row));
    if (path[2] == 'cancellation-eligibility') {
      return ok(eligibility(row, cancellation: true));
    }
    if (method == 'POST' && ['submit', 'cancel'].contains(path[2])) {
      if (row['version'] != body['expected_version']) {
        return fail('Request changed. Refresh before continuing.');
      }
      if (path[2] == 'submit') {
        if (row['status'] != 'DRAFT') {
          return fail('Only draft requests can be submitted.');
        }
        row['status'] = 'SUBMITTED';
        row['version']++;
        return ok(null);
      }
      if (['CANCELLED', 'CONVERTED', 'COMPLETED'].contains(row['status'])) {
        return fail('Cancellation is restricted.');
      }
      final decision = eligibility(row, cancellation: true);
      if (decision['state'] == 'RESTRICTED') {
        return fail('Cancellation is restricted.');
      }
      if (decision['state'] == 'REQUIRES_REVIEW' &&
          (body['review_approved'] != true ||
              '${body['reason'] ?? ''}'.trim().isEmpty)) {
        return fail('Review approval and a reason are required.');
      }
      for (final q in quotes.where((q) => q['request_id'] == row['id'])) {
        release(q['reservation_id']);
      }
      for (final j in production.jobs.where(
        (j) =>
            quotes.any(
              (q) => q['id'] == j['source_ref'] && q['request_id'] == row['id'],
            ) &&
            !['CANCELLED', 'COMPLETED'].contains(j['status']),
      )) {
        j['status'] = 'CANCELLED';
        j['version']++;
      }
      row['status'] = 'CANCELLED';
      row['version']++;
      return ok({'status': 'CANCELLED', 'eligibility_at_decision': decision});
    }
    return fail(
      'This pastry action is not available in this Mock workflow.',
      status: 400,
    );
  }

  JsonObject eligibility(JsonObject row, {bool cancellation = false}) {
    final now = DateTime.now().toUtc(),
        terminal = [
          'CANCELLED',
          'CONVERTED',
          'COMPLETED',
          'DECLINED',
          'EXPIRED',
        ].contains(row['status']);
    final policy = jsonObject(
          row[cancellation ? 'cancellation_policy' : 'change_policy'] ?? {},
        ),
        required = DateTime.tryParse('${row['required_at'] ?? ''}');
    DateTime? cutoff(String key) {
      final minutes = policy[key] as int? ?? 0;
      return required == null || minutes <= 0
          ? null
          : required.subtract(Duration(minutes: minutes));
    }

    final reached = (milestones['${row['id']}'] ?? [])
        .where(
          (m) =>
              ['ACTIVE', 'BLOCKED', 'READY', 'COMPLETED'].contains(m['state']),
        )
        .toList();
    final position = reached.isEmpty
        ? -1
        : reached
              .map((m) => m['position'] as int)
              .reduce((a, b) => a > b ? a : b);
    const order = [
      'DESIGN_APPROVED',
      'BASE_PREPARED',
      'DECORATION',
      'CHILLING',
      'PACKED',
      'READY',
    ];
    if (cancellation) {
      final review = cutoff('review_minutes_before_required_at'),
          restrict = cutoff('restrict_minutes_before_required_at');
      final restricted =
              terminal ||
              restrict != null && !now.isBefore(restrict) ||
              position >=
                  order.indexOf(
                    '${policy['restrict_after_milestone'] ?? 'DECORATION'}',
                  ) ||
              production.jobs.any(
                (j) =>
                    quotes.any(
                      (q) =>
                          q['id'] == j['source_ref'] &&
                          q['request_id'] == row['id'],
                    ) &&
                    ['IN_PROGRESS', 'READY', 'COMPLETED'].contains(j['status']),
              ),
          needsReview =
              row['status'] == 'ACCEPTED_PENDING_ORDER' ||
              review != null && !now.isBefore(review) ||
              position >=
                  order.indexOf(
                    '${policy['review_after_milestone'] ?? 'BASE_PREPARED'}',
                  );
      return {
        'state': restricted
            ? 'RESTRICTED'
            : needsReview
            ? 'REQUIRES_REVIEW'
            : 'ALLOWED',
        'reason_code': terminal
            ? 'REQUEST_TERMINAL'
            : restricted
            ? 'PRODUCTION_RESTRICTED'
            : needsReview
            ? 'REVIEW_REQUIRED'
            : 'BEFORE_PRODUCTION',
        'evaluated_at': now.toIso8601String(),
        if (review != null) 'review_at': review.toIso8601String(),
        if (restrict != null) 'restrict_at': restrict.toIso8601String(),
      };
    }
    final explicit = DateTime.tryParse('${row['change_cutoff_at'] ?? ''}'),
        derived = cutoff('minutes_before_required_at');
    final limit = explicit == null
        ? derived
        : derived == null || explicit.isBefore(derived)
        ? explicit
        : derived;
    final expired = limit != null && !now.isBefore(limit);
    final blocked =
        position >=
        order.indexOf(
          '${policy['disallowed_after_milestone'] ?? 'DECORATION'}',
        );
    return {
      'allowed': !terminal && !expired && !blocked,
      'reason_code': terminal
          ? 'REQUEST_TERMINAL'
          : expired
          ? 'CUTOFF_PASSED'
          : blocked
          ? 'MILESTONE_CUTOFF'
          : 'ALLOWED',
      'cutoff_at': limit?.toIso8601String(),
      'evaluated_at': now.toIso8601String(),
    };
  }

  void bumpAttached(String id, {bool option = false}) {
    final group = option
        ? options.entries
              .where((e) => e.value.any((o) => o['id'] == id))
              .map((e) => e.key)
              .firstOrNull
        : id;
    for (final entry in attachments.entries) {
      if (entry.value.containsKey(group)) {
        final product = find(products, entry.key);
        if (product != null) product['version']++;
      }
    }
  }

  JsonObject reserve(
    JsonObject r,
    List<JsonObject> allocations,
    String start,
    String end,
    String source,
    String ref, {
    int hold = 0,
    String? exclude,
  }) {
    final body = <String, dynamic>{
      'branch_id': r['branch_id'],
      'capability': 'PASTRY_BAKERY',
      'window_start': start,
      'window_end': end,
      'allocations': allocations,
      'source_type': source,
      'source_ref': ref,
    };
    final check = production.capacity.preview(body, exclude: exclude);
    if (check['_message'] != null) return check;
    if (jsonObject(check['_data'])['available'] != true) {
      return fail('Capacity is unavailable for this production window.');
    }
    final id = next('reservation');
    final row = <String, dynamic>{
      'id': id,
      ...body,
      'allocations': production.capacity.allocations(body),
      'priority': 'NORMAL',
      'status': hold > 0 ? 'HELD' : 'CONFIRMED',
      'hold_expires_at': hold > 0
          ? DateTime.now()
                .toUtc()
                .add(Duration(minutes: hold))
                .toIso8601String()
          : null,
      'created_at': now,
      'updated_at': now,
      'version': 1,
    };
    production.capacity.reservations[id] = row;
    production.capacity.revision++;
    return ok(row);
  }

  void release(Object? id, {String status = 'CANCELLED'}) {
    final row = production.capacity.reservations[id];
    if (row == null || !['HELD', 'CONFIRMED'].contains(row['status'])) return;
    row['status'] = status;
    row['version']++;
    row['updated_at'] = now;
    for (final a in (row['allocations'] as List).map(jsonObject)) {
      a['released_quantity'] = a['quantity'];
    }
    production.capacity.revision++;
  }

  String job(
    JsonObject r,
    String source,
    String ref,
    String reservation,
    int quantity,
    String start,
    String end,
  ) {
    final id = next('job');
    production.jobs.add({
      'id': id,
      'branch_id': r['branch_id'],
      'capability': 'PASTRY_BAKERY',
      'reservation_id': reservation,
      'source_type': source,
      'source_ref': ref,
      'source_version': r['version'],
      'title': source == 'PASTRY_PRODUCT'
          ? 'Pastry batch: ${r['name']}'
          : 'Custom cake $ref',
      'status': 'PLANNED',
      'priority': 'NORMAL',
      'planned_quantity': quantity,
      'produced_quantity': 0,
      'waste_quantity': 0,
      'blocked_reason': '',
      'scheduled_start': start,
      'scheduled_end': end,
      'version': 1,
      'created_at': now,
      'updated_at': now,
    });
    production.events[id] = [
      {
        'id': next('job-event'),
        'event_type': 'CREATED',
        'from_status': null,
        'to_status': 'PLANNED',
        'note': 'Pastry production created',
        'occurred_at': now,
      },
    ];
    return id;
  }

  String? effectiveSpec(String id) {
    final proposed = changes[id] ?? [];
    return (specs[id] ?? [])
            .where(
              (s) => !proposed.any(
                (c) =>
                    c['to_revision'] == s['revision'] &&
                    c['status'] != 'APPROVED',
              ),
            )
            .lastOrNull?['id']
        as String?;
  }

  bool fresh(JsonObject q) =>
      DateTime.tryParse(
        '${q['expires_at']}',
      )?.isAfter(DateTime.now().toUtc()) ==
      true;
  JsonObject? operational(
    String method,
    List<String> path,
    JsonObject body,
    Map<String, String> query,
  ) {
    if (path.first == 'products' &&
        path.length == 3 &&
        ['capacity-preview', 'production-batches'].contains(path[2])) {
      final r = find(products, path[1]);
      if (r == null) return fail('Product not found.', status: 404);
      if (r['status'] != 'ACTIVE' || r['capacity_pool_id'] == null) {
        return fail('Choose an active capacity-backed product.');
      }
      final quantity = body['quantity'];
      if (quantity is! int || quantity <= 0) {
        return fail('Enter a positive quantity.', status: 400);
      }
      final allocation = <String, dynamic>{
        'capacity_pool_id': r['capacity_pool_id'],
        'quantity': quantity * (r['workload_units_per_item'] as int),
        if ('${body['production_line_id'] ?? ''}'.isNotEmpty)
          'production_line_id': body['production_line_id'],
      };
      final capacityBody = <String, dynamic>{
        'branch_id': r['branch_id'],
        'capability': 'PASTRY_BAKERY',
        'window_start': body['window_start'],
        'window_end': body['window_end'],
        'allocations': [allocation],
        'source_type': 'PASTRY_PRODUCT',
        'source_ref': r['id'],
      };
      if (path.last == 'capacity-preview') {
        return production.capacity.preview(capacityBody);
      }
      final reserved = reserve(
        r,
        [allocation],
        '${body['window_start']}',
        '${body['window_end']}',
        'PASTRY_PRODUCT',
        '${r['id']}',
      );
      if (reserved['_message'] != null) return reserved;
      final id = '${jsonObject(reserved['_data'])['id']}';
      return ok({
        'production_job_id': job(
          r,
          'PASTRY_PRODUCT',
          '${r['id']}',
          id,
          quantity,
          '${body['window_start']}',
          '${body['window_end']}',
        ),
        'reservation_id': id,
      }, status: 201);
    }
    if (path.first == 'quotes' && path.length >= 2) {
      final q = find(quotes, path[1]);
      if (q == null) return fail('Quote not found.', status: 404);
      if (method == 'GET') return ok(q);
      final r = find(requests, '${q['request_id']}')!;
      if (path.last == 'accept' && q['status'] == 'ACCEPTED' && fresh(q)) {
        return ok({
          'conversion_intent_id': intents['${q['id']}'],
          'reservation_id': q['reservation_id'],
          'order_authority': 'NOT_PRESENT',
        });
      }
      if (body['expected_version'] != q['version']) {
        return fail('Quote changed. Refresh before continuing.');
      }
      if (path.last == 'revise') {
        if (!['DRAFT', 'PENDING_APPROVAL', 'APPROVED'].contains(q['status'])) {
          return fail('Quote cannot be revised.');
        }
        if (body['specification_version_id'] != effectiveSpec('${r['id']}')) {
          return fail('Select the current approved specification.');
        }
        final expires = DateTime.tryParse('${body['expires_at']}');
        if (expires == null ||
            !expires.isAfter(DateTime.now()) ||
            body['amount_minor'] is! int ||
            body['amount_minor'] < 0) {
          return fail('Check the price and expiry.', status: 400);
        }
        final id = next('quote');
        final replacement = <String, dynamic>{
          ...q,
          'id': id,
          'revision':
              quotes.where((x) => x['request_id'] == r['id']).length + 1,
          'version': 1,
          'status': q['approval_required'] == true
              ? 'PENDING_APPROVAL'
              : 'APPROVED',
          'amount_minor': body['amount_minor'],
          'expires_at': body['expires_at'],
          'specification_version_id': body['specification_version_id'],
          'vendor_terms': body['vendor_terms'] ?? '',
          'negotiation_reason': body['reason_code'] ?? '',
          'negotiation_note': body['note'] ?? '',
          'reservation_id': null,
          'approved_by': null,
          'approved_at': null,
        };
        final held = production.capacity.reservations[q['reservation_id']];
        final allocations = (q['capacity_requirements'] as List)
            .map(jsonObject)
            .toList();
        final limit = DateTime.now().toUtc().add(const Duration(minutes: 30));
        final holdExpiry = expires.isBefore(limit) ? expires : limit;
        if (held?['status'] == 'HELD' &&
            DateTime.tryParse(
                  '${held?['hold_expires_at']}',
                )?.isAfter(DateTime.now()) ==
                true) {
          held!['source_ref'] = id;
          held['hold_expires_at'] = holdExpiry.toIso8601String();
          held['version']++;
          held['updated_at'] = now;
          production.capacity.revision++;
          replacement['reservation_id'] = held['id'];
        } else if (allocations.isNotEmpty) {
          final remaining = holdExpiry.difference(DateTime.now()).inMinutes;
          final reserved = reserve(
            r,
            allocations,
            '${q['production_start']}',
            '${q['production_end']}',
            'CUSTOM_CAKE_QUOTE',
            id,
            hold: remaining < 1 ? 1 : remaining,
          );
          if (reserved['_message'] != null) return reserved;
          replacement['reservation_id'] = jsonObject(reserved['_data'])['id'];
        }
        q['status'] = 'SUPERSEDED';
        q['version']++;
        quotes.add(replacement);
        r['status'] = replacement['approval_required'] == true
            ? 'QUOTE_PENDING'
            : 'QUOTED';
        r['version']++;
        return ok({'id': id}, status: 201);
      }
      if (path.last == 'decline') {
        if (!['DRAFT', 'PENDING_APPROVAL', 'APPROVED'].contains(q['status'])) {
          return fail('Quote cannot be declined.');
        }
        release(q['reservation_id']);
        q['status'] = 'DECLINED';
        q['version']++;
        r['status'] = 'DECLINED';
        r['version']++;
        return ok(null);
      }
      if (!fresh(q)) return fail('Quote has expired.');
      if (q['specification_version_id'] != effectiveSpec('${r['id']}')) {
        return fail('The quoted specification has been superseded.');
      }
      if (path.last == 'approve') {
        if (!['DRAFT', 'PENDING_APPROVAL'].contains(q['status'])) {
          return fail('Quote cannot be approved.');
        }
        q['status'] = 'APPROVED';
        q['approved_by'] = 'mock-staff-owner';
        q['approved_at'] = now;
        q['version']++;
        r['status'] = 'QUOTED';
        r['version']++;
        return ok(null);
      }
      if (path.last == 'accept') {
        if (q['status'] != 'APPROVED') {
          return fail('Only approved quotes can be accepted.');
        }
        final allocations = (q['capacity_requirements'] as List)
            .map(jsonObject)
            .toList();
        if (allocations.isEmpty) {
          return fail('Quote has no production capacity requirements.');
        }
        var reservation = q['reservation_id'];
        final held = production.capacity.reservations[reservation];
        if (held != null) {
          if (held['status'] != 'HELD' ||
              !DateTime.parse(
                held['hold_expires_at'],
              ).isAfter(DateTime.now())) {
            return fail('The capacity hold has expired.');
          }
          held['status'] = 'CONFIRMED';
          held['hold_expires_at'] = null;
          held['version']++;
          production.capacity.revision++;
        } else {
          final reserved = reserve(
            r,
            allocations,
            '${q['production_start']}',
            '${q['production_end']}',
            'CUSTOM_CAKE_QUOTE',
            '${q['id']}',
          );
          if (reserved['_message'] != null) return reserved;
          reservation = jsonObject(reserved['_data'])['id'];
        }
        job(
          {...r, 'version': q['revision']},
          'CUSTOM_CAKE_QUOTE',
          '${q['id']}',
          '$reservation',
          1,
          '${q['production_start']}',
          '${q['production_end']}',
        );
        q['reservation_id'] = reservation;
        q['status'] = 'ACCEPTED';
        q['accepted_at'] = now;
        q['version']++;
        r['status'] = 'ACCEPTED_PENDING_ORDER';
        r['version']++;
        milestones['${r['id']}'] = [
          for (final type in [
            'DESIGN_APPROVED',
            'BASE_PREPARED',
            'DECORATION',
            'CHILLING',
            'PACKED',
            'READY',
          ])
            {
              'id': next('milestone'),
              'type': type,
              'state': type == 'DESIGN_APPROVED' ? 'ACTIVE' : 'PENDING',
              'position': [
                'DESIGN_APPROVED',
                'BASE_PREPARED',
                'DECORATION',
                'CHILLING',
                'PACKED',
                'READY',
              ].indexOf(type),
              'version': 1,
            },
        ];
        final intent = next('conversion');
        intents['${q['id']}'] = intent;
        return ok({
          'conversion_intent_id': intent,
          'reservation_id': reservation,
          'order_authority': 'NOT_PRESENT',
        });
      }
    }
    if (path.first == 'changes' && path.length >= 3) {
      final change = changes.values
          .expand((v) => v)
          .where((c) => c['id'] == path[1])
          .firstOrNull;
      if (change == null) return fail('Change not found.', status: 404);
      if (method == 'GET') return ok(histories['change:${path[1]}'] ?? []);
      if (change['version'] != body['expected_version'] ||
          change['status'] != 'OPEN') {
        return fail('This change has already been decided or updated.');
      }
      if (!['APPROVED', 'DECLINED'].contains(body['decision'])) {
        return fail('Choose a valid decision.', status: 400);
      }
      change['status'] = body['decision'];
      change['version']++;
      change['decision_note'] = body['note'] ?? '';
      change['decided_at'] = now;
      histories.putIfAbsent('change:${path[1]}', () => []).add({
        'from_status': 'OPEN',
        'to_status': body['decision'],
        'actor_id': 'mock-staff-owner',
        'note': body['note'] ?? '',
        'occurred_at': now,
      });
      return ok({'id': change['id'], 'status': body['decision']});
    }
    if (path.first != 'custom-requests' || path.length < 3) return null;
    final r = find(requests, path[1]);
    if (r == null) return fail('Request not found.', status: 404);
    if (path[2] == 'specifications' &&
        path.length == 4 &&
        path.last == 'diff') {
      final all = specs[path[1]] ?? [],
          from = find(all, query['from'] ?? ''),
          to = find(all, query['to'] ?? '');
      if (from == null || to == null) {
        return fail('Specification not found.', status: 404);
      }
      final before = jsonObject(from['specification']),
          after = jsonObject(to['specification']);
      return ok([
        for (final key in {...before.keys, ...after.keys})
          if (jsonEncode(before[key]) != jsonEncode(after[key]))
            {'field': key, 'before': before[key], 'after': after[key]},
      ]);
    }
    if (path[2] == 'quotes') {
      if (method == 'GET') {
        return ok(
          quotes
              .where((q) => q['request_id'] == r['id'])
              .map((q) => {...q}..remove('capacity_requirements'))
              .toList(),
        );
      }
      if (r['version'] != body['expected_request_version']) {
        return fail('Request changed. Refresh before quoting.');
      }
      if ([
        'CANCELLED',
        'CONVERTED',
        'COMPLETED',
        'ACCEPTED_PENDING_ORDER',
      ].contains(r['status'])) {
        return fail('Request cannot be quoted.');
      }
      if (quotes.any(
        (q) =>
            q['request_id'] == r['id'] &&
            ['DRAFT', 'PENDING_APPROVAL', 'APPROVED'].contains(q['status']),
      )) {
        return fail('An active quote already exists. Revise it instead.');
      }
      if (body['specification_version_id'] != effectiveSpec('${r['id']}')) {
        return fail('Select the current approved specification.');
      }
      final expires = DateTime.tryParse('${body['expires_at']}');
      if (body['amount_minor'] is! int ||
          body['amount_minor'] < 0 ||
          expires == null ||
          !expires.isAfter(DateTime.now())) {
        return fail('Check the price and expiry.', status: 400);
      }
      final allocations = (body['capacity_allocations'] as List? ?? [])
          .map(jsonObject)
          .toList();
      if (allocations.isEmpty && body['capacity_pool_id'] != null) {
        allocations.add({
          'capacity_pool_id': body['capacity_pool_id'],
          'quantity': 1,
        });
      }
      final id = next('quote');
      String? reservation;
      if (allocations.isNotEmpty) {
        var hold = body['hold_for_minutes'] as int? ?? 0;
        final remaining = expires.difference(DateTime.now()).inMinutes;
        if (hold <= 0 || hold > remaining) {
          hold = remaining < 30 ? remaining : 30;
        }
        if (hold < 1) hold = 1;
        final reserved = reserve(
          r,
          allocations,
          '${body['production_start']}',
          '${body['production_end']}',
          'CUSTOM_CAKE_QUOTE',
          id,
          hold: hold,
        );
        if (reserved['_message'] != null) return reserved;
        reservation = jsonObject(reserved['_data'])['id'];
      }
      final approval = body['approval_required'] as bool? ?? true;
      quotes.add({
        'id': id,
        'request_id': r['id'],
        'revision': quotes.where((q) => q['request_id'] == r['id']).length + 1,
        'specification_version_id': body['specification_version_id'],
        'amount_minor': body['amount_minor'],
        'currency': 'NGN',
        'expires_at': body['expires_at'],
        'status': approval ? 'PENDING_APPROVAL' : 'APPROVED',
        'reservation_id': reservation,
        'production_start': body['production_start'],
        'production_end': body['production_end'],
        'capacity_requirements': allocations,
        'approval_required': approval,
        'approved_by': null,
        'approved_at': null,
        'vendor_terms': body['vendor_terms'] ?? '',
        'negotiation_reason': body['negotiation_reason'] ?? '',
        'negotiation_note': body['negotiation_note'] ?? '',
        'accepted_at': null,
        'version': 1,
        'created_at': now,
      });
      r['status'] = approval ? 'QUOTE_PENDING' : 'QUOTED';
      r['version']++;
      return ok({'id': id, 'reservation_id': reservation ?? ''}, status: 201);
    }
    if (path[2] == 'changes') {
      final list = changes.putIfAbsent(path[1], () => []);
      if (method == 'GET') return ok(list);
      if (r['version'] != body['expected_version'] ||
          eligibility(r)['allowed'] != true) {
        return fail('This request changed or is outside its change window.');
      }
      final history = specs.putIfAbsent(path[1], () => []),
          rev = history.length + 1,
          id = next('change'),
          spec = next('specification');
      history.add({
        'id': spec,
        'revision': rev,
        'specification': body['specification'],
        'created_at': now,
      });
      list.add({
        'id': id,
        'request_id': r['id'],
        'from_revision': rev - 1,
        'to_revision': rev,
        'reason': body['reason'],
        'status': 'OPEN',
        'version': 1,
        'created_at': now,
        'decision_note': '',
      });
      r['version']++;
      return ok({
        'change_request_id': id,
        'specification_id': spec,
        'revision': rev,
      }, status: 201);
    }
    if (path[2] == 'milestones') {
      final list = milestones[path[1]] ?? [];
      if (path.length == 3) return ok(list);
      final m = list.where((m) => m['type'] == path[3]).firstOrNull;
      if (m == null) return fail('Milestone not found.', status: 404);
      final key = 'milestone:${path[1]}:${path[3]}';
      if (method == 'GET') return ok(histories[key] ?? []);
      final state = path[4], from = '${m['state']}';
      if (body['expected_version'] != m['version']) {
        return fail('Milestone changed. Refresh before continuing.');
      }
      final allowed =
          state == from ||
          (state == 'BLOCKED' || state == 'CANCELLED') && from != 'COMPLETED' ||
          from == 'PENDING' && state == 'ACTIVE' ||
          from == 'BLOCKED' && state == 'ACTIVE' ||
          from == 'ACTIVE' &&
              (state == 'COMPLETED' ||
                  m['type'] == 'READY' && state == 'READY') ||
          from == 'READY' && state == 'COMPLETED';
      if (!allowed) return fail('Invalid milestone transition.');
      if (state == 'ACTIVE' &&
          list.any(
            (prev) =>
                prev['position'] < m['position'] &&
                !['COMPLETED', 'READY'].contains(prev['state']),
          )) {
        return fail('Complete the previous milestone first.');
      }
      if (state == from) return ok({'version': m['version']});
      m['state'] = state;
      m['version']++;
      histories.putIfAbsent(key, () => []).add({
        'id': next('history'),
        'from_state': from,
        'to_state': state,
        'note': body['note'] ?? '',
        'actor_id': 'mock-staff-owner',
        'changed_at': now,
      });
      if (m['type'] == 'READY' && ['READY', 'COMPLETED'].contains(state)) {
        for (final j in production.jobs.where(
          (j) =>
              j['source_type'] == 'CUSTOM_CAKE_QUOTE' &&
              quotes.any(
                (q) => q['id'] == j['source_ref'] && q['request_id'] == r['id'],
              ) &&
              !['CANCELLED', 'COMPLETED', 'READY'].contains(j['status']),
        )) {
          final before = j['status'];
          j['status'] = 'READY';
          j['version']++;
          production.events.putIfAbsent('${j['id']}', () => []).add({
            'id': next('job-event'),
            'event_type': 'READY',
            'from_status': before,
            'to_status': 'READY',
            'note': 'Pastry READY milestone reached',
            'occurred_at': now,
          });
        }
      }
      return ok({'version': m['version']});
    }
    return null;
  }
}
