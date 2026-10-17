import 'mock_intelligence_store.dart';
import 'mock_export_store.dart';
import 'mock_campaign_store.dart';
import 'dart:convert';
import 'package:http/http.dart' as http;
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'mock_onboarding_store.dart';
import 'mock_settings_store.dart';

class MockCrmStore {
  MockCrmStore(this.business, this.settings, {required bool empty}) {
    if (empty) return;
    profiles.add({
      'profile_ref': 'mock-crm-customer-1',
      'subject_type': 'CUSTOMER',
      'display_label': 'Ada Okafor',
      'status': 'ACTIVE',
      'privacy_state': 'ACTIVE',
      'merge_state': 'NONE',
      'created_at': now,
      'version': 1,
    });
    profiles.add({
      'profile_ref': 'mock-crm-customer-2',
      'subject_type': 'GUEST',
      'display_label': 'Guest relationship',
      'status': 'ACTIVE',
      'privacy_state': 'ACTIVE',
      'merge_state': 'NONE',
      'created_at': now,
      'version': 1,
    });
  }
  late final exports = MockExportStore(this);
  late final intelligence = MockIntelligenceStore(this);
  final MockOnboardingStore business;
  final MockSettingsStore settings;
  late final campaigns = MockCampaignStore(this);
  final customerRefs = <String, String>{
    'mock-crm-customer-1': 'mock-customer-celebration',
  };
  final profiles = <JsonObject>[];
  final rows = <String, List<JsonObject>>{};
  final members = <String, Set<String>>{},
      exclusions = <String, Set<String>>{},
      tagMembers = <String, Set<String>>{};
  final revisions = <String, List<JsonObject>>{},
      histories = <String, List<JsonObject>>{};
  final ledgers = <String, List<JsonObject>>{};
  final balances = <String, int>{},
      credited = <String, int>{},
      debited = <String, int>{};
  final replay = <String, ({String fingerprint, JsonObject result})>{};
  JsonObject program = {
    'status': 'DISABLED',
    'points_name_singular': 'point',
    'points_name_plural': 'points',
    'expiry_policy': 'NEVER',
    'minimum_redemption_points': 0,
    'redemption_increment_points': 1,
    'manual_adjustment_limit': 1000,
    'configured': false,
    'version': 0,
  };
  JsonObject tiers = {'tiers': <JsonObject>[], 'version': 0};
  int sequence = 0;
  String get now => DateTime.now().toUtc().toIso8601String();
  JsonObject ok(Object? data, [int status = 200]) => {
    '_data': jsonDecode(jsonEncode(data)),
    '_status': status,
  };
  JsonObject fail(String message, [int status = 422]) => {
    '_message': message,
    '_code': 'CRM_REQUEST_REFUSED',
    '_status': status,
  };
  List<JsonObject> bucket(String key) => rows.putIfAbsent(key, () => []);
  JsonObject? profile(String? id) =>
      profiles.where((p) => p['profile_ref'] == id).firstOrNull;
  bool actionable(JsonObject p) =>
      p['privacy_state'] == 'ACTIVE' &&
      p['status'] == 'ACTIVE' &&
      p['merge_state'] != 'MERGED_SOURCE';
  bool validStaff(Object? id) =>
      id == null ||
      id == '' ||
      settings.staff.any((r) => r['id'] == id && r['status'] == 'active');
  bool validBranch(Object? id) =>
      id == null || id == '' || business.branches.any((r) => r['id'] == id);
  JsonObject commercial() => {'state': 'NOT_AVAILABLE'};
  JsonObject consent() => {
    'integration_available': false,
    'marketing': {
      for (final channel in ['EMAIL', 'PUSH', 'IN_APP', 'SMS'])
        channel: {
          'decision': 'UNKNOWN',
          'eligible': false,
          'reason': 'CONSENT_SOURCE_UNAVAILABLE',
        },
    },
  };
  JsonObject loyalty(String id) => {
    'program_status': program['status'],
    'enrollment': balances.containsKey(id) ? 'ENROLLED' : 'NOT_ENROLLED',
    'points_name_singular': program['points_name_singular'],
    'points_name_plural': program['points_name_plural'],
    'available_points': balances[id] ?? 0,
    'lifetime_credited_points': credited[id] ?? 0,
    'lifetime_debited_points': debited[id] ?? 0,
    'pending_expiry_points': 0,
    if (balances.containsKey(id)) 'account_status': 'ACTIVE',
  };
  JsonObject customer(JsonObject p) => {
    'profile': p,
    'commercial': commercial(),
    'branches': [],
    'capability': {'affinity_state': 'NOT_AVAILABLE'},
    'consent': consent(),
    'privacy': {'privacy_state': p['privacy_state']},
    'identity_link': {'merge_state': p['merge_state']},
    'tags': [
      for (final t in bucket('tags'))
        if (tagMembers[t['tag_ref']]?.contains(p['profile_ref']) == true) t,
    ],
    'segments': [],
    'loyalty': loyalty(p['profile_ref']),
    'modules': {
      'tags': 'AVAILABLE',
      'segments': 'AVAILABLE',
      'recent_interactions': 'AVAILABLE',
      'open_tasks': 'AVAILABLE',
      'loyalty': 'AVAILABLE',
    },
  };
  String idKey(String kind) => switch (kind) {
    'customers' => 'profile_ref',
    'tags' => 'tag_ref',
    'segments' => 'segment_ref',
    'interactions' => 'interaction_ref',
    'tasks' => 'task_ref',
    'corporate-customers' => 'corporate_ref',
    'contacts' => 'contact_ref',
    'locations' => 'location_ref',
    'cost-centers' => 'cost_center_ref',
    'memberships' => 'membership_ref',
    _ => 'id',
  };
  JsonObject page(
    List<JsonObject> items,
    Map<String, String> q, {
    String wrapper = 'items',
  }) {
    final offset = int.tryParse(q['offset'] ?? '') ?? 0,
        limit = (int.tryParse(q['limit'] ?? '') ?? 50).clamp(1, 200);
    final filtered = items
        .where(
          (r) =>
              (q['status'] == null || r['status'] == q['status']) &&
              (q['priority'] == null || r['priority'] == q['priority']) &&
              (q['customer_profile_ref'] == null ||
                  r['customer_profile_ref'] == q['customer_profile_ref']) &&
              (q['branch_ref'] == null || r['branch_ref'] == q['branch_ref']) &&
              (q['search'] == null ||
                  jsonEncode(
                    r,
                  ).toLowerCase().contains(q['search']!.toLowerCase())),
        )
        .toList();
    return ok({
      wrapper: filtered.skip(offset).take(limit).toList(),
      'has_more': filtered.length > offset + limit,
      'limit': limit,
      'offset': offset,
      'pagination': {
        'has_more': filtered.length > offset + limit,
        'limit': limit,
        'offset': offset,
      },
    });
  }

  JsonObject? handle(http.BaseRequest request, JsonObject body) {
    if (!request.url.path.startsWith('/api/v1/vendor/crm/')) return null;
    final p = request.url.path
        .replaceFirst('/api/v1/vendor/crm/', '')
        .split('/');
    if (p.first == 'campaigns') return campaigns.handle(request, body);
    if (p.first == 'exports') return exports.handle(request, body, p);
    if (p.first == 'intelligence') {
      return intelligence.handle(request, body, p);
    }
    if (request.method == 'GET') return read(p, request.url.queryParameters);
    final key =
        request.headers['Idempotency-Key'] ??
        request.headers['idempotency-key'];
    final customerState =
        p.first == 'customers' && ['archive', 'restore'].contains(p.last);
    if (!customerState && (key == null || key.isEmpty)) {
      return fail('A retry key is required.', 400);
    }
    final fingerprint = jsonEncode([
      request.method,
      request.url.toString(),
      body,
      request.headers['If-Match'] ?? request.headers['if-match'],
    ]);
    final saved = key == null ? null : replay[key];
    if (!customerState && saved != null) {
      return saved.fingerprint == fingerprint
          ? saved.result
          : fail('This key belongs to another change.', 409);
    }
    final result = write(p, request, body);
    if (!customerState && key != null && (result['_status'] as int) < 300) {
      replay[key] = (fingerprint: fingerprint, result: result);
    }
    return result;
  }

  JsonObject read(List<String> p, Map<String, String> q) {
    if (p.first == 'segment-fields') {
      return ok({
        'schema_version': 1,
        'limits': {
          'max_nodes': 50,
          'max_depth': 6,
          'max_or_children': 25,
          'max_in_values': 100,
        },
        'fields': [
          {
            'key': 'profile.status',
            'label': 'Relationship status',
            'type': 'ENUM',
            'source': 'PROFILE',
            'scopes': ['BUSINESS', 'BRANCH'],
            'operators': ['EQ', 'NEQ', 'IN', 'NOT_IN'],
            'values': ['ACTIVE', 'ARCHIVED'],
            'available': true,
            'branch_only': false,
          },
          {
            'key': 'profile.subject_type',
            'label': 'Subject type',
            'type': 'ENUM',
            'source': 'PROFILE',
            'scopes': ['BUSINESS', 'BRANCH'],
            'operators': ['EQ', 'NEQ', 'IN', 'NOT_IN'],
            'values': ['CUSTOMER', 'GUEST', 'EXTERNAL'],
            'available': true,
            'branch_only': false,
          },
          {
            'key': 'commercial.completed_order_count',
            'label': 'Completed order count',
            'type': 'INTEGER',
            'source': 'COMMERCIAL',
            'scopes': ['BUSINESS', 'BRANCH'],
            'operators': ['EQ', 'GTE', 'GT', 'LT', 'LTE', 'BETWEEN'],
            'available': false,
            'branch_only': false,
          },
        ],
      });
    }
    if (p.first == 'customers') {
      if (p.length == 1) {
        return page(
          profiles
              .where((p) => q['status'] == null || p['status'] == q['status'])
              .map(
                (p) => {
                  'profile': p,
                  'commercial': commercial(),
                  'marketing_eligibility': {
                    'integration_available': false,
                    'eligible_channels': [],
                    'any_channel_eligible': false,
                  },
                },
              )
              .toList(),
          {...q}..remove('status'),
        );
      }
      final record = profile(p[1]);
      if (record == null || record['privacy_state'] == 'REDACTED') {
        return fail('Customer relationship not found.', 404);
      }
      if (p.length == 2) return ok(customer(record));
      if (p[2] == 'consent') return ok(consent());
      if (p[2] == 'privacy') {
        return ok({
          'privacy_state': record['privacy_state'],
          'marketing_suppressed': false,
        });
      }
      if (p[2] == 'intelligence') {
        return ok({'profile_ref': p[1], 'state': 'NOT_AVAILABLE'});
      }
      if (p[2] == 'loyalty') {
        if (p.length == 3) return ok(loyalty(p[1]));
        return page(
          (ledgers[p[1]] ?? []).reversed.toList(),
          q,
          wrapper: 'entries',
        );
      }
      if (['interactions', 'tasks'].contains(p[2])) {
        return page(
          bucket(p[2]).where((r) => r['customer_profile_ref'] == p[1]).toList(),
          q,
        );
      }
    }
    if (p.first == 'loyalty') {
      if (p[1] == 'program') return ok(program);
      if (p[1] == 'tiers') return ok(tiers);
    }
    if (p.first == 'corporate-customers') {
      if (p.length == 1) {
        return page(
          bucket('corporate-customers'),
          q,
          wrapper: 'corporate_customers',
        );
      }
      final account = bucket(
        'corporate-customers',
      ).where((r) => r['corporate_ref'] == p[1]).firstOrNull;
      if (account == null) return fail('Corporate account not found.', 404);
      if (p.length == 2) {
        return ok({
          'corporate_customer': account,
          'summary': {
            'contact_count': bucket('corporate/${p[1]}/contacts').length,
            'location_count': bucket('corporate/${p[1]}/locations').length,
            'cost_center_count': bucket(
              'corporate/${p[1]}/cost-centers',
            ).length,
            'membership_count': bucket('corporate/${p[1]}/memberships').length,
            'open_task_count': bucket('corporate/${p[1]}/tasks')
                .where((r) => ['OPEN', 'IN_PROGRESS'].contains(r['status']))
                .length,
            'overdue_task_count': 0,
            'commercial': commercial(),
            'rfq': {'state': 'NOT_AVAILABLE'},
          },
          'recent_interactions': bucket(
            'corporate/${p[1]}/interactions',
          ).reversed.take(5).toList(),
        });
      }
      if (p[2] == 'branches') {
        return ok({
          'corporate_ref': p[1],
          'scope_mode': account['scope_mode'],
          'branch_refs': account['branch_refs'],
          'version': account['version'],
        });
      }
      final list = bucket('corporate/${p[1]}/${p[2]}');
      if (p.length == 3) {
        return page(list, q, wrapper: p[2].replaceAll('-', '_'));
      }
      final r = list.where((r) => r[idKey(p[2])] == p[3]).firstOrNull;
      return r == null ? fail('Corporate detail not found.', 404) : ok(r);
    }
    final list = bucket(p.first);
    if (p.length == 1) return page(list, q);
    final record = list.where((r) => r[idKey(p.first)] == p[1]).firstOrNull;
    if (record == null) return fail('CRM record not found.', 404);
    if (p.length == 2) return ok(record);
    if (p.last == 'revisions') return ok({'items': revisions[p[1]] ?? []});
    if (p.last == 'history') return ok({'items': histories[p[1]] ?? []});
    if (p.first == 'segments' && p.last == 'members') {
      return page([
        for (final id in members[p[1]] ?? <String>{})
          if (profile(id) != null && actionable(profile(id)!))
            {
              'profile_ref': id,
              'display_label': profile(id)!['display_label'],
              'membership_type': record['segment_type'],
              'added_at': now,
            },
      ], q);
    }
    return fail('CRM route not found.', 404);
  }

  void bump(JsonObject r) {
    r['version'] = (r['version'] as int) + 1;
    r['updated_at'] = now;
  }

  bool sameVersion(JsonObject r, JsonObject b) =>
      b['expected_version'] == r['version'];
  JsonObject write(List<String> p, http.BaseRequest request, JsonObject b) {
    if (p.first == 'customers') {
      final target = profile(p[1]);
      if (target == null) return fail('Customer relationship not found.', 404);
      if (p.last == 'archive' || p.last == 'restore') {
        if (target['privacy_state'] != 'ACTIVE' ||
            target['merge_state'] == 'MERGED_SOURCE') {
          return fail('This relationship cannot be changed.', 409);
        }
        final version = int.tryParse(
          request.headers['If-Match'] ??
              request.headers['if-match'] ??
              request.url.queryParameters['expected_version'] ??
              '',
        );
        if (version != target['version']) {
          return fail('Relationship changed. Refresh before editing.', 409);
        }
        target['status'] = p.last == 'archive' ? 'ARCHIVED' : 'ACTIVE';
        bump(target);
        return ok(target);
      }
      if (p.last == 'adjustments') return adjust(p[1], b);
    }
    if (p.first == 'loyalty') {
      if (p[1] == 'program') {
        if (!sameVersion(program, b)) {
          return fail('Programme changed. Refresh before saving.', 409);
        }
        final next = {...program, ...b}..remove('expected_version');
        if (!['DISABLED', 'ACTIVE', 'PAUSED'].contains(next['status']) ||
            !['NEVER', 'FIXED_DAYS'].contains(next['expiry_policy']) ||
            next['redemption_increment_points'] is! int ||
            next['redemption_increment_points'] < 1) {
          return fail('Check programme details.');
        }
        if (next['expiry_policy'] == 'FIXED_DAYS' &&
            (next['expiry_days'] is! int || next['expiry_days'] < 1)) {
          return fail('Fixed expiry needs a positive number of days.');
        }
        if (b['clear_expiry_days'] == true) next.remove('expiry_days');
        next.remove('clear_expiry_days');
        next['configured'] = true;
        bump(next);
        program = next;
        return ok(program);
      }
      if (p[1] == 'tiers') {
        if (tiers['version'] > 0 && !sameVersion(tiers, b)) {
          return fail('Tier ladder changed.', 409);
        }
        final next = (b['tiers'] as List? ?? []).map(jsonObject).toList();
        if (next.length > 8) return fail('Use at most eight tiers.');
        final keys = <String>{}, thresholds = <int>{};
        for (final r in next) {
          if (!RegExp(
                r'^[A-Z][A-Z0-9_]{1,39}$',
              ).hasMatch('${r['tier_key']}'.toUpperCase()) ||
              r['display_name'] == null ||
              r['display_name'] == '' ||
              r['minimum_lifetime_points'] is! int ||
              r['minimum_lifetime_points'] < 0 ||
              !keys.add(r['tier_key']) ||
              !thresholds.add(r['minimum_lifetime_points'])) {
            return fail('Each tier needs a unique key and threshold.');
          }
        }
        next.sort(
          (a, b) => (a['minimum_lifetime_points'] as int).compareTo(
            b['minimum_lifetime_points'],
          ),
        );
        tiers = {
          'tiers': next,
          'version': tiers['version'] + 1,
          'updated_at': now,
        };
        return ok(tiers);
      }
      if (p[1] == 'entries' && p.last == 'reverse') return reverse(p[2], b);
    }
    if (p.first == 'segments' && p.length > 1 && p[1] == 'preview') {
      return preview(b);
    }
    if (p.first == 'tags' &&
        p.length > 1 &&
        ['bulk-assign', 'bulk-remove'].contains(p[1])) {
      return bulkTag(b, p[1] == 'bulk-assign');
    }
    if (p.first == 'corporate-customers' && p.length > 2) {
      return corporateWrite(p, request, b);
    }
    final list = bucket(p.first),
        create = p.length == 1 && request.method == 'POST',
        key = idKey(p.first);
    final r = create ? null : list.where((r) => r[key] == p[1]).firstOrNull;
    if (!create && r == null) return fail('CRM record not found.', 404);
    if (create) {
      if (['interactions', 'tasks'].contains(p.first)) {
        final profileRow = profile(b['customer_profile_ref']);
        if (profileRow == null || !actionable(profileRow)) {
          return fail('Choose an active customer relationship.', 409);
        }
        if (!validBranch(b['branch_ref']) ||
            b['visibility'] == 'BRANCH' &&
                (b['branch_ref'] == null || b['branch_ref'] == '')) {
          return fail('Choose a branch for branch visibility.');
        }
        if (!validStaff(b['assigned_staff_ref'])) {
          return fail('Choose an active staff member.');
        }
      }
      if (['tags', 'segments'].contains(p.first)) {
        final scope = b['scope'] as Map?;
        if (scope == null ||
            !['BUSINESS', 'BRANCH'].contains(scope['type']) ||
            scope['type'] == 'BRANCH' &&
                (scope['branch_ref'] == null ||
                    !validBranch(scope['branch_ref']))) {
          return fail('Choose a valid scope.');
        }
        if (b['name'] == null || b['name'] == '') {
          return fail('A name is required.');
        }
      }
      if (p.first == 'corporate-customers' &&
          (b['organization_name'] == null ||
              b['organization_name'] == '' ||
              !validStaff(b['account_manager_staff_ref']))) {
        return fail('Choose an organisation and valid account manager.');
      }
      if (p.first == 'segments' && b['segment_type'] == 'DYNAMIC') {
        final checked = preview(b);
        if (checked['_message'] != null) return checked;
      }
      final record = <String, dynamic>{
        key: 'mock-crm-${p.first}-${++sequence}',
        ...b,
        'version': 1,
        'created_at': now,
        'updated_at': now,
      };
      if (p.first == 'tags' || p.first == 'segments') {
        final scope = b['scope'] as Map;
        record.remove('scope');
        record['scope_type'] = scope['type'];
        if (scope['branch_ref'] != null) {
          record['branch_ref'] = scope['branch_ref'];
        }
        record['status'] = p.first == 'tags' ? 'ACTIVE' : 'DRAFT';
      }
      if (p.first == 'segments') {
        record.addAll({
          'definition_version': 1,
          'evaluation_status': 'NOT_EVALUATED',
          'member_count': 0,
        });
        members[record[key]] = {};
        exclusions[record[key]] = {};
      }
      if (p.first == 'tasks') {
        if (b['title'] == null || b['title'] == '') {
          return fail('A title is required.');
        }
        record['status'] = 'OPEN';
        record['is_overdue'] = false;
        history(record, 'CREATED');
      }
      if (p.first == 'interactions') {
        if (!validInteraction(b)) {
          return fail(
            'Check interaction type, direction, outcome and timestamp.',
          );
        }
        revisions[record[key]] = [revision(record)];
      }
      if (p.first == 'corporate-customers') {
        record['status'] = 'PROSPECT';
        record['scope_mode'] = b['scope_mode'] ?? 'BUSINESS';
        record['branch_refs'] = b['branch_refs'] ?? [];
        record['corporate_customer_code'] =
            'CORP-${sequence.toString().padLeft(4, '0')}';
      }
      list.add(record);
      return ok(record, 201);
    }
    if (p.first == 'segments' &&
        p.length > 3 &&
        ['members', 'exclusions'].contains(p[2])) {
      return bulkSegment(r!, p[2], b, p.last == 'bulk-add');
    }
    if (p.first == 'segments' && p.last == 'materialize') {
      if (r!['segment_type'] != 'DYNAMIC' || r['status'] == 'ARCHIVED') {
        return fail('Only active dynamic definitions can be evaluated.', 409);
      }
      final result = preview({
        'rule': r['rule'],
        'scope': {'type': r['scope_type'], 'branch_ref': r['branch_ref']},
      });
      if (result['_message'] != null) return result;
      members[r[key]] =
          profiles
              .where(actionable)
              .where((p) => matches((r['rule'] as Map)['root'] as Map, p))
              .map((p) => p['profile_ref'] as String)
              .toSet()
            ..removeAll(exclusions[r[key]] ?? {});
      r['evaluation_status'] = 'CURRENT';
      r['member_count'] = members[r[key]]!.length;
      r['last_evaluated_at'] = now;
      r['materialized_definition_version'] = r['definition_version'];
      return ok({
        'segment_ref': r[key],
        'status': 'QUEUED',
        'run_ref': 'mock-crm-evaluation-${++sequence}',
      });
    }
    if (!sameVersion(r!, b)) {
      return fail('This record changed. Refresh before saving.', 409);
    }
    if (p.first == 'tasks') return mutateTask(r, p, b);
    if (p.first == 'interactions' && p.last == 'correct') return correct(r, b);
    if (p.first == 'corporate-customers' && request.method == 'POST') {
      final target = switch (p.last) {
        'activate' => 'ACTIVE',
        'hold' => 'ON_HOLD',
        'deactivate' => 'INACTIVE',
        'archive' => 'ARCHIVED',
        _ => null,
      };
      final allowed = switch (r['status']) {
        'PROSPECT' => ['ACTIVE', 'ARCHIVED'],
        'ACTIVE' => ['ON_HOLD', 'INACTIVE', 'ARCHIVED'],
        'ON_HOLD' => ['ACTIVE', 'INACTIVE', 'ARCHIVED'],
        'INACTIVE' => ['ACTIVE', 'ARCHIVED'],
        _ => [],
      };
      if (!allowed.contains(target)) {
        return fail('Corporate state transition is unavailable.', 409);
      }
      r['status'] = target;
      bump(r);
      return ok(r);
    }
    if (p.last == 'archive') {
      r['status'] = 'ARCHIVED';
      bump(r);
      return ok(r);
    }
    if (p.last == 'activate') {
      if (r['status'] != 'DRAFT') {
        return fail('Only draft segments can be activated.', 409);
      }
      r['status'] = 'ACTIVE';
      bump(r);
      return ok(r);
    }
    if (request.method == 'PATCH') {
      if (r['status'] == 'ARCHIVED') {
        return fail('Archived records cannot be edited.', 409);
      }
      if (b['rule'] != null) {
        final validation = preview({
          'rule': b['rule'],
          'scope': {'type': r['scope_type'], 'branch_ref': r['branch_ref']},
        });
        if (validation['_message'] != null) return validation;
        r['definition_version'] += 1;
        r['evaluation_status'] = 'STALE';
      }
      for (final e in b.entries) {
        if (![
              'expected_version',
              'scope',
              'scope_mode',
              'branch_refs',
              'change_reason',
            ].contains(e.key) &&
            e.value != null) {
          r[e.key] = e.value;
        }
      }
      if (b['clear_account_manager'] == true) {
        r.remove('account_manager_staff_ref');
      }
      r.remove('clear_account_manager');
      bump(r);
      return ok(r);
    }
    return fail('CRM action unavailable.', 404);
  }

  bool validInteraction(JsonObject b) =>
      [
        'NOTE',
        'PHONE_CALL',
        'IN_PERSON',
        'EMAIL',
        'SMS',
        'IN_APP',
        'SOCIAL',
        'SALES_FOLLOW_UP',
        'SERVICE_FOLLOW_UP',
        'FEEDBACK',
        'ACCOUNT_REVIEW',
        'OTHER',
      ].contains(b['type']) &&
      [
        'INBOUND',
        'OUTBOUND',
        'INTERNAL',
        'NOT_APPLICABLE',
      ].contains(b['direction']) &&
      [
        'NONE',
        'SUCCESSFUL',
        'NO_RESPONSE',
        'FOLLOW_UP_REQUIRED',
        'RESOLVED',
        'UNRESOLVED',
        'CUSTOMER_DECLINED',
        'VENDOR_DECLINED',
        'ESCALATED',
        'CANCELLED',
      ].contains(b['outcome']) &&
      DateTime.tryParse('${b['occurred_at']}') != null;
  JsonObject revision(JsonObject r) => {
    'revision_ref': 'mock-crm-revision-${++sequence}',
    for (final key in ['version', 'subject', 'note', 'outcome', 'occurred_at'])
      key: r[key],
    'created_at': now,
  };
  JsonObject correct(JsonObject r, JsonObject b) {
    if (b['correction_reason'] == null || b['correction_reason'] == '') {
      return fail('A correction reason is required.');
    }
    for (final key in ['subject', 'note', 'outcome', 'occurred_at']) {
      if (b[key] != null) r[key] = b[key];
    }
    bump(r);
    revisions.putIfAbsent(r['interaction_ref'], () => []).add({
      ...revision(r),
      'correction_reason': b['correction_reason'],
    });
    return ok(r);
  }

  void history(JsonObject r, String action) {
    histories.putIfAbsent(r['task_ref'], () => []).add({
      'history_ref': 'mock-crm-task-history-${++sequence}',
      'action': action,
      for (final key in [
        'title',
        'description',
        'status',
        'priority',
        'assigned_staff_ref',
        'due_at',
        'version',
      ])
        key: r[key],
      'occurred_at': now,
    });
  }

  JsonObject mutateTask(JsonObject r, List<String> p, JsonObject b) {
    if (['COMPLETED', 'CANCELLED'].contains(r['status'])) {
      return fail('Completed or cancelled tasks cannot be changed.', 409);
    }
    final target = switch (p.last) {
      'start' => 'IN_PROGRESS',
      'complete' => 'COMPLETED',
      'cancel' => 'CANCELLED',
      _ => null,
    };
    if (target == 'IN_PROGRESS' && r['status'] != 'OPEN') {
      return fail('Only open tasks can be started.', 409);
    }
    if (b.containsKey('assigned_staff_ref') &&
        !validStaff(b['assigned_staff_ref'])) {
      return fail('Choose an active assignee.');
    }
    for (final key in [
      'title',
      'description',
      'priority',
      'due_at',
      'assigned_staff_ref',
      'contact_ref',
      'location_ref',
      'cost_center_ref',
      'source_interaction_ref',
    ]) {
      if (b.containsKey(key) && b[key] != null) r[key] = b[key];
    }
    if (b['clear_due_at'] == true) r.remove('due_at');
    if (target != null) r['status'] = target;
    bump(r);
    history(r, target ?? 'UPDATED');
    return ok(r);
  }

  JsonObject bulkTag(JsonObject b, bool add) {
    final tag = bucket(
      'tags',
    ).where((r) => r['tag_ref'] == b['tag_ref']).firstOrNull;
    if (tag == null || tag['status'] != 'ACTIVE') {
      return fail('Choose an active tag.', 409);
    }
    final refs = List<String>.from(b['profile_refs'] as List? ?? []).toSet();
    if (refs.isEmpty || refs.length > 500) {
      return fail('Select 1..500 customers.');
    }
    if (refs.any((id) => profile(id) == null || !actionable(profile(id)!))) {
      return fail(
        'A selected relationship is unavailable. Nothing was changed.',
        409,
      );
    }
    final set = tagMembers.putIfAbsent(tag['tag_ref'], () => {});
    var applied = 0;
    for (final id in refs) {
      if (add ? set.add(id) : set.remove(id)) applied++;
    }
    tag['member_count'] = set.length;
    return ok({
      'tag_ref': tag['tag_ref'],
      'requested': refs.length,
      'applied': applied,
      'skipped': refs.length - applied,
    });
  }

  JsonObject bulkSegment(JsonObject r, String kind, JsonObject b, bool add) {
    if (r['status'] == 'ARCHIVED' ||
        kind == 'members' && r['segment_type'] != 'STATIC' ||
        kind == 'exclusions' && r['segment_type'] != 'DYNAMIC') {
      return fail('Membership action unavailable.', 409);
    }
    final refs = List<String>.from(b['profile_refs'] as List? ?? []).toSet();
    if (refs.isEmpty || refs.length > 500) {
      return fail('Select 1..500 customers.');
    }
    if (refs.any((id) => profile(id) == null || !actionable(profile(id)!))) {
      return fail(
        'A selected relationship is unavailable. Nothing was changed.',
        409,
      );
    }
    final set = (kind == 'members' ? members : exclusions).putIfAbsent(
      r['segment_ref'],
      () => {},
    );
    var applied = 0;
    for (final id in refs) {
      if (add ? set.add(id) : set.remove(id)) applied++;
    }
    r['member_count'] = members[r['segment_ref']]?.length ?? 0;
    if (kind == 'exclusions') {
      r['evaluation_status'] = 'STALE';
    }
    return ok({
      'tag_ref': r['segment_ref'],
      'requested': refs.length,
      'applied': applied,
      'skipped': refs.length - applied,
    });
  }

  bool matches(Map node, JsonObject p) {
    if (node['op'] != null) {
      final children = (node['children'] as List).cast<Map>();
      return node['op'] == 'AND'
          ? children.every((n) => matches(n, p))
          : node['op'] == 'OR'
          ? children.any((n) => matches(n, p))
          : !matches(children.single, p);
    }
    final value =
        p[node['field'] == 'profile.status' ? 'status' : 'subject_type'];
    return switch (node['operator']) {
      'EQ' => value == node['value'],
      'NEQ' => value != node['value'],
      'IN' => (node['value'] as List).contains(value),
      'NOT_IN' => !(node['value'] as List).contains(value),
      _ => false,
    };
  }

  bool ruleValid(Map node) {
    if (node['op'] != null) {
      return ['AND', 'OR', 'NOT'].contains(node['op']) &&
          node['children'] is List &&
          (node['children'] as List).isNotEmpty &&
          (node['children'] as List).every((n) => n is Map && ruleValid(n));
    }
    return ['profile.status', 'profile.subject_type'].contains(node['field']) &&
        ['EQ', 'NEQ', 'IN', 'NOT_IN'].contains(node['operator']) &&
        (['IN', 'NOT_IN'].contains(node['operator'])
            ? node['value'] is List
            : node['value'] is String);
  }

  JsonObject preview(JsonObject b) {
    final rule = b['rule'];
    if (rule is! Map ||
        rule['schema_version'] != 1 ||
        rule['root'] is! Map ||
        !ruleValid(rule['root'])) {
      return fail('This Mock rule uses an unavailable or unsupported field.');
    }
    final matched = profiles
        .where(actionable)
        .where((p) => matches(rule['root'], p))
        .toList();
    return ok({
      'match_count': matched.length,
      'count_state': 'EXACT',
      'scope': (b['scope'] as Map?)?['type'] ?? 'BUSINESS',
      'evaluation_as_of': now,
      'rule_explanation': 'Mock relationship rule',
      'sample': [
        for (final p in matched.take(b['sample_limit'] as int? ?? 10))
          {
            'profile_ref': p['profile_ref'],
            'display_label': p['display_label'],
          },
      ],
      'unavailable_fields': [],
      'warnings': [],
    });
  }

  JsonObject adjust(String id, JsonObject b) {
    final p = profile(id);
    if (p == null || !actionable(p)) {
      return fail('Relationship cannot receive points.', 409);
    }
    if (program['status'] != 'ACTIVE') {
      return fail(
        'Activate the loyalty programme before adjusting points.',
        409,
      );
    }
    final points = b['points'];
    if (points is! int ||
        points < 1 ||
        points > program['manual_adjustment_limit'] ||
        b['reason'] == null ||
        b['reason'] == '') {
      return fail('Check points and adjustment reason.');
    }
    if (!['CREDIT', 'DEBIT'].contains(b['direction'])) {
      return fail('Choose credit or debit.');
    }
    final delta = b['direction'] == 'CREDIT' ? points : -points;
    if ((balances[id] ?? 0) + delta < 0) {
      return fail('There are not enough points.', 409);
    }
    balances[id] = (balances[id] ?? 0) + delta;
    if (delta > 0) {
      credited[id] = (credited[id] ?? 0) + delta;
    } else {
      debited[id] = (debited[id] ?? 0) - delta;
    }
    final entry = {
      'entry_ref': 'mock-crm-ledger-${++sequence}',
      'sequence': (ledgers[id]?.length ?? 0) + 1,
      'entry_type': delta > 0 ? 'ADJUSTMENT_CREDIT' : 'ADJUSTMENT_DEBIT',
      'delta_points': delta,
      'balance_after': balances[id],
      'source_type': 'MANUAL_ADJUSTMENT',
      'reason_code': b['reason'],
      'created_at': now,
    };
    ledgers.putIfAbsent(id, () => []).add(entry);
    return ok({'entry': entry, 'summary': loyalty(id)}, 201);
  }

  JsonObject reverse(String entryId, JsonObject b) {
    for (final item in ledgers.entries) {
      final entry = item.value
          .where((e) => e['entry_ref'] == entryId)
          .firstOrNull;
      if (entry == null) continue;
      final p = profile(item.key);
      if (p == null || !actionable(p)) {
        return fail('Relationship is unavailable.', 409);
      }
      if (entry['reversed_by_entry_ref'] != null ||
          ![
            'ADJUSTMENT_CREDIT',
            'ADJUSTMENT_DEBIT',
          ].contains(entry['entry_type'])) {
        return fail('This entry cannot be reversed.', 409);
      }
      if (b['reason'] == null || b['reason'] == '') {
        return fail('A reason is required.');
      }
      final delta = -(entry['delta_points'] as int);
      if ((balances[item.key] ?? 0) + delta < 0) {
        return fail('Credited points have already been consumed.', 409);
      }
      balances[item.key] = (balances[item.key] ?? 0) + delta;
      final reversal = {
        'entry_ref': 'mock-crm-ledger-${++sequence}',
        'sequence': item.value.length + 1,
        'entry_type': 'REVERSAL',
        'delta_points': delta,
        'balance_after': balances[item.key],
        'source_type': 'REVERSAL',
        'reverses_entry_ref': entryId,
        'reason_code': b['reason'],
        'created_at': now,
      };
      entry['reversed_by_entry_ref'] = reversal['entry_ref'];
      item.value.add(reversal);
      return ok({'entry': reversal, 'summary': loyalty(item.key)});
    }
    return fail('Ledger entry not found.', 404);
  }

  JsonObject corporateWrite(
    List<String> p,
    http.BaseRequest request,
    JsonObject b,
  ) {
    final account = bucket(
      'corporate-customers',
    ).where((r) => r['corporate_ref'] == p[1]).firstOrNull;
    if (account == null) return fail('Corporate account not found.', 404);
    if (account['status'] == 'ARCHIVED') {
      return fail('Archived corporate accounts cannot be changed.', 409);
    }
    if (p[2] == 'branches') {
      if (!sameVersion(account, b)) {
        return fail('Corporate account changed.', 409);
      }
      final refs = List<String>.from(b['branch_refs'] as List? ?? []);
      if (!['BUSINESS', 'BRANCH_SET'].contains(b['scope_mode']) ||
          refs.any((id) => !validBranch(id)) ||
          b['scope_mode'] == 'BRANCH_SET' && refs.isEmpty) {
        return fail('Choose a valid branch scope.');
      }
      account['scope_mode'] = b['scope_mode'];
      account['branch_refs'] = b['scope_mode'] == 'BUSINESS' ? [] : refs;
      bump(account);
      return ok(account);
    }
    final kind = p[2],
        key = idKey(kind),
        list = bucket('corporate/${p[1]}/$kind');
    final create = p.length == 3 && request.method == 'POST';
    final r = create ? null : list.where((r) => r[key] == p[3]).firstOrNull;
    if (!create && r == null) return fail('Corporate detail not found.', 404);
    if (!create && !sameVersion(r!, b)) {
      return fail('Corporate detail changed.', 409);
    }
    if (kind == 'contacts' && create) {
      final target = profile(b['customer_profile_ref']);
      if (target == null || !actionable(target)) {
        return fail('Choose an active customer profile.');
      }
      if (list.any(
        (r) =>
            r['customer_profile_ref'] == target['profile_ref'] &&
            r['status'] != 'ARCHIVED',
      )) {
        return fail('This contact already belongs to the account.', 409);
      }
    }
    for (final pair in [
      ('contact_ref', 'contacts'),
      ('location_ref', 'locations'),
      ('cost_center_ref', 'cost-centers'),
    ]) {
      final id = b[pair.$1];
      if (id != null &&
          id != '' &&
          !bucket(
            'corporate/${p[1]}/${pair.$2}',
          ).any((r) => r[idKey(pair.$2)] == id && r['status'] == 'ACTIVE')) {
        return fail('Choose an active reference from this corporate account.');
      }
    }
    if (!validStaff(b['assigned_staff_ref'])) {
      return fail('Choose an active staff member.');
    }
    if (create) {
      if (kind == 'memberships' &&
          (b['contact_ref'] == null || b['contact_ref'] == '')) {
        return fail('A contact is required.');
      }
      if (['locations', 'cost-centers'].contains(kind) &&
          (b['name'] == null || b['name'] == '')) {
        return fail('A name is required.');
      }
      final record = <String, dynamic>{
        key: 'mock-crm-corporate-$kind-${++sequence}',
        'corporate_ref': p[1],
        ...b,
        'version': 1,
        'created_at': now,
        'updated_at': now,
      };
      if (!['interactions', 'tasks'].contains(kind)) {
        record['status'] = b['status'] ?? 'ACTIVE';
      }
      if (kind == 'tasks') {
        if (b['title'] == null || b['title'] == '') {
          return fail('A title is required.');
        }
        record['status'] = 'OPEN';
        record['is_overdue'] = false;
      }
      if (kind == 'interactions') {
        record['occurred_at'] = b['occurred_at'] ?? now;
        if (!validInteraction(record)) {
          return fail('Check interaction details.');
        }
      }
      if (kind == 'contacts') {
        record['display_label'] = profile(
          b['customer_profile_ref'],
        )!['display_label'];
        record['privacy_suppressed'] = false;
        record['actionable'] = true;
        if (record['is_primary_contact'] == true) {
          for (final existing in list) {
            existing['is_primary_contact'] = false;
          }
        }
      }
      list.add(record);
      return ok(record, 201);
    }
    if (kind == 'tasks') return mutateTask(r!, p, b);
    if (kind == 'interactions' && p.last == 'correct') return correct(r!, b);
    if (p.last == 'archive') {
      r!['status'] = 'ARCHIVED';
      bump(r);
      return ok(r);
    }
    for (final e in b.entries) {
      if (![
            'expected_version',
            'customer_profile_ref',
            'contact_ref',
          ].contains(e.key) &&
          !e.key.startsWith('clear_') &&
          e.value != null) {
        r![e.key] = e.value;
      }
    }
    if (b['clear_location'] == true) r!.remove('location_ref');
    if (b['clear_cost_center'] == true) r!.remove('cost_center_ref');
    bump(r!);
    return ok(r);
  }
}
