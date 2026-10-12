import 'package:crypto/crypto.dart';
import 'dart:convert';
import 'package:http/http.dart' as http;
import '../../core/contracts/vendor_contract.dart';
import 'mock_crm_store.dart';

class MockCampaignStore {
  MockCampaignStore(this.crm) {
    if (crm.profiles.isEmpty) return;
    final segment = {
      'segment_ref': 'mock-campaign-segment',
      'name': 'Customer relationships',
      'description': 'Mock campaign audience',
      'type': 'STATIC',
      'status': 'ACTIVE',
      'scope': {'type': 'BUSINESS'},
      'definition_version': 1,
      'version': 1,
      'membership_count': crm.profiles.length,
      'created_at': crm.now,
    };
    crm.bucket('segments').add(segment);
    crm.members['mock-campaign-segment'] = {
      for (final p in crm.profiles) '${p['profile_ref']}',
    };
    final body = <String, dynamic>{
      'name': 'Stay connected',
      'objective': 'RETENTION',
      'scope_type': 'BUSINESS',
      'channels': ['IN_APP'],
      'source_segment_ref': 'mock-campaign-segment',
      'exclusion_segment_refs': <String>[],
      'service_recovery_suppression_enabled': true,
      'quiet_hours_enabled': false,
      'experiment_enabled': false,
      'experiment_variants': <JsonObject>[],
    };
    final seeded = jsonObject(create(body)['_data']);
    seedHistory('${seeded['campaign']['campaign_ref']}');
  }
  void seedHistory(String campaignRef) {
    final snapshotRef = id('snapshot'), runRef = id('run');
    final people = crm.profiles;
    final targetRows = <JsonObject>[
      for (final person in people)
        {
          'target_ref': id('target'),
          'profile_ref': person['profile_ref'],
          'display_label': person['display_label'],
          'is_control': false,
          'currently_actionable': true,
          'service_recovery_active': false,
          'channels': [
            {
              'channel': 'IN_APP',
              'snapshot_consent_decision': 'ALLOW',
              'snapshot_consent_eligible': true,
              'snapshot_eligibility_state': 'ELIGIBLE_AT_SNAPSHOT',
              'reason_code': 'CONSENT_ALLOWED',
              'send_time_recheck_required': true,
            },
          ],
        },
    ];
    targets[snapshotRef] = targetRows;
    snapshots[campaignRef] = [
      {
        'snapshot_ref': snapshotRef,
        'campaign_ref': campaignRef,
        'campaign_definition_version': 1,
        'definition_hash': 'mock-historical-definition',
        'status': 'SUPERSEDED',
        'evaluation_as_of': crm.now,
        'target_count': people.length,
        'source_count': people.length,
        'control_count': 0,
        'treatment_count': people.length,
        'send_time_consent_recheck_required': true,
        'send_time_frequency_recheck_required': true,
        'send_time_quiet_hours_recheck_required': true,
        'created_at': crm.now,
      },
    ];
    final ds = <JsonObject>[
      for (final t in targetRows)
        {
          'delivery_ref': id('delivery'),
          'run_ref': runRef,
          'campaign_ref': campaignRef,
          'snapshot_ref': snapshotRef,
          'target_ref': t['target_ref'],
          'channel': 'IN_APP',
          'is_control': false,
          'status': 'SUPPRESSED',
          'suppression_reason': 'CONSENT_SOURCE_UNAVAILABLE',
          'consent_decision_at_handoff': 'UNKNOWN',
          'consent_eligible_at_handoff': false,
          'consent_checked_at': crm.now,
          'policy_checked_at': crm.now,
          'version': 1,
          'created_at': crm.now,
        },
    ];
    deliveries[runRef] = ds;
    for (final d in ds) {
      histories['${d['delivery_ref']}'] = [
        {
          'history_ref': id('history'),
          'delivery_ref': d['delivery_ref'],
          'to_status': 'SUPPRESSED',
          'from_status': 'PENDING',
          'source': 'WORKER',
          'reason_code': 'CONSENT_SOURCE_UNAVAILABLE',
          'occurred_at': crm.now,
          'recorded_at': crm.now,
        },
      ];
    }
    runs[campaignRef] = [
      {
        'run_ref': runRef,
        'campaign_ref': campaignRef,
        'snapshot_ref': snapshotRef,
        'status': 'COMPLETED',
        'mode': 'SEND_NOW',
        'scheduled_for': crm.now,
        'total_deliveries': ds.length,
        'handed_off_count': 0,
        'delivered_count': 0,
        'suppressed_count': ds.length,
        'failed_count': 0,
        'version': 1,
        'created_at': crm.now,
      },
    ];
  }

  final MockCrmStore crm;
  final campaigns = <JsonObject>[];
  final definitions = <String, JsonObject>{};
  final snapshots = <String, List<JsonObject>>{},
      targets = <String, List<JsonObject>>{},
      runs = <String, List<JsonObject>>{},
      deliveries = <String, List<JsonObject>>{},
      histories = <String, List<JsonObject>>{};
  final replay = <String, ({String fingerprint, JsonObject result})>{};
  int sequence = 0;
  String id(String kind) => 'mock-campaign-$kind-${++sequence}';
  JsonObject? find(List<JsonObject> items, String key, String value) =>
      items.where((r) => r[key] == value).firstOrNull;
  JsonObject response(Object? data, [int status = 200]) => crm.ok(data, status);
  JsonObject fail(String message, [int status = 422]) =>
      crm.fail(message, status);
  JsonObject handle(http.BaseRequest request, JsonObject body) {
    final p = request.url.path
        .replaceFirst('/api/v1/vendor/crm/campaigns', '')
        .split('/')
        .where((s) => s.isNotEmpty)
        .toList();
    if (request.method == 'GET') return read(p, request.url.queryParameters);
    final preview = p.lastOrNull == 'preview';
    final key =
        request.headers['Idempotency-Key'] ??
        request.headers['idempotency-key'];
    final retryable =
        p.isEmpty ||
        p.length == 1 ||
        ['archive', 'prepare'].contains(p.last) ||
        p.length == 2 && p.last == 'runs';
    if (retryable && (key == null || key.isEmpty)) {
      return fail('A retry key is required.', 400);
    }
    final fingerprint = jsonEncode([
      request.method,
      request.url.toString(),
      body,
    ]);
    final saved = key == null ? null : replay[key];
    if (saved != null) {
      return saved.fingerprint == fingerprint
          ? saved.result
          : fail('This key belongs to another command.', 409);
    }
    final result = preview
        ? previewCampaign(p.first, body)
        : write(p, request.method, body);
    if (retryable && key != null && (result['_status'] as int) < 300) {
      replay[key] = (fingerprint: fingerprint, result: result);
    }
    return result;
  }

  JsonObject read(List<String> p, Map<String, String> q) {
    if (p.isEmpty) return crm.page(campaigns, q, wrapper: 'campaigns');
    final campaign = find(campaigns, 'campaign_ref', p.first);
    if (campaign == null) return fail('Campaign not found.', 404);
    if (p.length == 1) {
      return response({
        'campaign': campaign,
        'definition': definitions[p.first],
      });
    }
    if (p[1] == 'snapshots') {
      final list = snapshots[p.first] ?? [];
      if (p.length == 2) return crm.page(list, q, wrapper: 'snapshots');
      final snapshot = find(list, 'snapshot_ref', p[2]);
      if (snapshot == null) return fail('Snapshot not found.', 404);
      if (p.length == 3) return response(snapshot);
      var listTargets = targets[p[2]] ?? [];
      if (q['actionable_only'] == 'true') {
        listTargets = listTargets
            .where((r) => r['currently_actionable'] == true)
            .toList();
      }
      if (q['cohort'] == 'CONTROL') {
        listTargets = listTargets
            .where((r) => r['is_control'] == true)
            .toList();
      }
      if (q['cohort'] == 'TREATMENT') {
        listTargets = listTargets
            .where((r) => r['is_control'] != true)
            .toList();
      }
      if (q['eligible_on_channel'] != null) {
        listTargets = listTargets
            .where(
              (r) => (r['channels'] as List).any(
                (v) =>
                    v['channel'] == q['eligible_on_channel'] &&
                    v['snapshot_consent_eligible'] == true,
              ),
            )
            .toList();
      }
      final page = crm.page(listTargets, q, wrapper: 'targets');
      final data = jsonObject(page['_data']);
      data['send_time_recheck_required'] = true;
      return response(data);
    }
    if (p[1] == 'runs') {
      final list = runs[p.first] ?? [];
      if (p.length == 2) return crm.page(list, q, wrapper: 'runs');
      final run = find(list, 'run_ref', p[2]);
      if (run == null) return fail('Run not found.', 404);
      if (p.length == 3) return response(run);
      final listDeliveries = deliveries[p[2]] ?? [];
      if (p.length == 4) {
        return crm.page(listDeliveries, q, wrapper: 'deliveries');
      }
      final delivery = find(listDeliveries, 'delivery_ref', p[4]);
      if (delivery == null) return fail('Delivery not found.', 404);
      if (p.length == 5) return response(delivery);
      return crm.page(histories[p[4]] ?? [], q, wrapper: 'history');
    }
    return fail('Campaign resource not found.', 404);
  }

  String? validateDefinition(JsonObject b) {
    if ('${b['name'] ?? ''}'.trim().length < 2 || '${b['name']}'.length > 120) {
      return 'Use a campaign name between 2 and 120 characters.';
    }
    if (![
      'GENERAL_MARKETING',
      'RETENTION',
      'REACTIVATION',
      'LOYALTY',
      'ANNOUNCEMENT',
    ].contains(b['objective'] ?? 'GENERAL_MARKETING')) {
      return 'Choose a supported objective.';
    }
    if (!['BUSINESS', 'BRANCH'].contains(b['scope_type'] ?? 'BUSINESS')) {
      return 'Choose a supported scope.';
    }
    if (b['scope_type'] == 'BRANCH' &&
        (b['branch_ref'] == null || !crm.validBranch(b['branch_ref']))) {
      return 'Choose an existing branch.';
    }
    final channels = List<String>.from(b['channels'] as List? ?? []);
    if (channels.isEmpty ||
        channels.length > 4 ||
        channels.toSet().length != channels.length ||
        channels.any((c) => !['EMAIL', 'PUSH', 'IN_APP', 'SMS'].contains(c))) {
      return 'Choose valid unique channels.';
    }
    final refs = [
      b['source_segment_ref'],
      ...b['exclusion_segment_refs'] as List? ?? [],
    ];
    if (refs.first == null ||
        refs.length > 11 ||
        refs.toSet().length != refs.length) {
      return 'Choose a source and up to ten distinct exclusion segments.';
    }
    for (final ref in refs) {
      final s = find(crm.bucket('segments'), 'segment_ref', '$ref');
      if (s == null || s['status'] != 'ACTIVE') {
        return 'The audience segment must be active.';
      }
      final scope = jsonObject(s['scope'] ?? {});
      if (b['scope_type'] == 'BUSINESS' && scope['type'] == 'BRANCH' ||
          b['scope_type'] == 'BRANCH' &&
              scope['type'] == 'BRANCH' &&
              scope['branch_ref'] != b['branch_ref']) {
        return 'The segment does not match the campaign scope.';
      }
    }
    final cap = b['frequency_cap_count'],
        window = b['frequency_cap_window_minutes'];
    if ((cap == null) != (window == null) ||
        cap != null && (cap is! int || cap < 1 || cap > 1000) ||
        window != null && (window is! int || window < 1 || window > 525600)) {
      return 'Set a valid contact cap and window together.';
    }
    if (b['quiet_hours_enabled'] == true) {
      if (!RegExp(
            r'^([01]\d|2[0-3]):[0-5]\d$',
          ).hasMatch('${b['quiet_hours_start']}') ||
          !RegExp(
            r'^([01]\d|2[0-3]):[0-5]\d$',
          ).hasMatch('${b['quiet_hours_end']}') ||
          b['quiet_hours_start'] == b['quiet_hours_end'] ||
          '${b['quiet_hours_timezone'] ?? ''}'.isEmpty) {
        return 'Complete valid quiet hours and a timezone.';
      }
    }
    if (b['experiment_enabled'] == true) {
      final v = (b['experiment_variants'] as List? ?? [])
          .map(jsonObject)
          .toList();
      final control = v.where((r) => r['is_control'] == true).toList(),
          treatment = v.where((r) => r['is_control'] != true).toList();
      if (v.length > 6 ||
          treatment.isEmpty ||
          treatment.length > 5 ||
          control.length > 1 ||
          control.any((r) => (r['allocation_bps'] as int? ?? -1) > 5000) ||
          v.any(
            (r) =>
                !RegExp(r'^[A-Z0-9_]{1,32}$').hasMatch('${r['variant_key']}') ||
                '${r['display_name'] ?? ''}'.isEmpty ||
                '${r['display_name']}'.length > 80 ||
                r['allocation_bps'] is! int ||
                (r['allocation_bps'] as int) < 0 ||
                (r['allocation_bps'] as int) > 10000,
          ) ||
          v.fold<int>(0, (s, r) => s + (r['allocation_bps'] as int? ?? 0)) !=
              10000 ||
          v.map((r) => r['variant_key']).toSet().length != v.length) {
        return 'Review the experiment variants and allocation.';
      }
    }
    return null;
  }

  JsonObject definition(JsonObject b, int version) => {
    'name': b['name'],
    'objective': b['objective'] ?? 'GENERAL_MARKETING',
    'scope_type': b['scope_type'] ?? 'BUSINESS',
    if (b['scope_type'] == 'BRANCH') 'branch_ref': b['branch_ref'],
    'channels': b['channels'],
    'source_segment_ref': b['source_segment_ref'],
    'exclusion_segment_refs': b['exclusion_segment_refs'] ?? [],
    'definition_version': version,
    'definition_hash': 'mock-definition-$version',
    'frequency_policy': {
      'enabled': b['frequency_cap_count'] != null,
      if (b['frequency_cap_count'] != null) 'cap': b['frequency_cap_count'],
      if (b['frequency_cap_window_minutes'] != null)
        'window_minutes': b['frequency_cap_window_minutes'],
      'evaluation_state': 'NOT_AVAILABLE',
      'send_time_recheck_required': true,
    },
    'quiet_hours_policy': {
      'enabled': b['quiet_hours_enabled'] ?? false,
      if (b['quiet_hours_enabled'] == true) ...{
        'start': b['quiet_hours_start'],
        'end': b['quiet_hours_end'],
        'timezone': b['quiet_hours_timezone'],
        'crosses_midnight':
            '${b['quiet_hours_start']}'.compareTo('${b['quiet_hours_end']}') >
            0,
      },
      'send_time_recheck_required': true,
    },
    'service_recovery_suppression_enabled':
        b['service_recovery_suppression_enabled'] ?? true,
    'experiment_enabled': b['experiment_enabled'] ?? false,
    'experiment_variants': b['experiment_enabled'] == true
        ? b['experiment_variants']
        : [],
    'created_at': crm.now,
    if (b['change_reason'] != null) 'change_reason': b['change_reason'],
  };
  JsonObject create(JsonObject b) {
    final ref = id('definition');
    final def = definition(b, 1);
    final campaign = <String, dynamic>{
      'campaign_ref': ref,
      'name': def['name'],
      'objective': def['objective'],
      'scope_type': def['scope_type'],
      if (def['branch_ref'] != null) 'branch_ref': def['branch_ref'],
      'status': 'DRAFT',
      'definition_version': 1,
      'version': 1,
      'created_at': crm.now,
      'updated_at': crm.now,
    };
    campaigns.insert(0, campaign);
    definitions[ref] = def;
    return response({'campaign': campaign, 'definition': def}, 201);
  }

  List<JsonObject>? audience(JsonObject def) {
    final ref = '${def['source_segment_ref']}';
    final segment = find(crm.bucket('segments'), 'segment_ref', ref);
    if (segment == null ||
        segment['status'] != 'ACTIVE' ||
        segment['type'] == 'DYNAMIC' &&
            segment['evaluation_status'] != 'CURRENT') {
      return null;
    }
    final excluded = <String>{};
    for (final e in def['exclusion_segment_refs'] as List? ?? []) {
      final s = find(crm.bucket('segments'), 'segment_ref', '$e');
      if (s == null ||
          s['status'] != 'ACTIVE' ||
          s['type'] == 'DYNAMIC' && s['evaluation_status'] != 'CURRENT') {
        return null;
      }
      excluded.addAll(crm.members['$e'] ?? {});
    }
    return crm.profiles
        .where(
          (p) =>
              (crm.members[ref] ?? {}).contains(p['profile_ref']) &&
              !excluded.contains(p['profile_ref']) &&
              crm.actionable(p),
        )
        .toList();
  }

  JsonObject assignment(String ref, JsonObject def, String profile) {
    if (def['experiment_enabled'] != true) return {'is_control': false};
    final digest = sha256
        .convert(
          utf8.encode(
            '$ref:${def['definition_version']}\x00$profile\x00vendor.crm.campaign.experiment.v1',
          ),
        )
        .bytes;
    var value = BigInt.zero;
    for (final byte in digest.take(8)) {
      value = (value << 8) + BigInt.from(byte);
    }
    final bucket = (value % BigInt.from(10000)).toInt();
    final variants =
        (def['experiment_variants'] as List).map(jsonObject).toList()..sort(
          (a, b) => a['is_control'] != b['is_control']
              ? a['is_control'] == true
                    ? -1
                    : 1
              : '${a['variant_key']}'.compareTo('${b['variant_key']}'),
        );
    var ceiling = 0;
    for (final variant in variants) {
      ceiling += variant['allocation_bps'] as int;
      if (bucket < ceiling) {
        return {
          'is_control': variant['is_control'],
          'experiment_variant_key': variant['variant_key'],
          'experiment_bucket': bucket,
        };
      }
    }
    throw const FormatException('Invalid campaign allocation');
  }

  List<JsonObject> channelCounts(JsonObject def, int count) => [
    for (final ch in def['channels'] as List)
      {
        'channel': ch,
        'consent_eligible_count': 0,
        'consent_ineligible_count': count,
        'consent_ineligible_reasons': {'CONSENT_SOURCE_UNAVAILABLE': count},
        'send_time_recheck_required': true,
      },
  ];
  JsonObject previewCampaign(String ref, JsonObject b) {
    final c = find(campaigns, 'campaign_ref', ref);
    if (c == null) return fail('Campaign not found.', 404);
    final def = definitions[ref]!, people = audience(def);
    final source = crm.members['${def['source_segment_ref']}'] ?? {};
    final excluded = {
      for (final e in def['exclusion_segment_refs'] as List? ?? [])
        ...crm.members['$e'] ?? <String>{},
    };
    final exclusions = source.where(excluded.contains).length;
    final count = people?.length ?? 0;
    final assignments = {
      for (final p in people ?? <JsonObject>[])
        '${p['profile_ref']}': assignment(ref, def, '${p['profile_ref']}'),
    };
    final control = assignments.values
        .where((a) => a['is_control'] == true)
        .length;
    return response({
      'campaign_ref': ref,
      'definition_version': def['definition_version'],
      'definition_hash': def['definition_hash'],
      'evaluation_as_of': crm.now,
      'scope_type': def['scope_type'],
      'source_segment_ref': def['source_segment_ref'],
      'source_segment_evaluation_status': people == null
          ? 'NOT_CURRENT'
          : 'CURRENT',
      'count_state': 'EXACT',
      'source_count': people == null ? 0 : source.length,
      'segment_exclusion_count': people == null ? 0 : exclusions,
      'privacy_suppressed_count': people == null
          ? 0
          : source.length - exclusions - count,
      'service_recovery_suppressed_count': 0,
      'consent_suppressed_count': count,
      'target_count': count,
      'control_count': control,
      'treatment_count': count - control,
      'experiment_variant_counts': {
        for (final v in (def['experiment_variants'] as List? ?? []))
          '${v['variant_key']}': assignments.values
              .where((a) => a['experiment_variant_key'] == v['variant_key'])
              .length,
      },
      'channels': channelCounts(def, count),
      'frequency_policy': def['frequency_policy'],
      'quiet_hours_policy': def['quiet_hours_policy'],
      'service_recovery_suppression_enabled':
          def['service_recovery_suppression_enabled'],
      'sample': [
        for (final p in (people ?? []).take(
          (b['sample_limit'] as int? ?? 25).clamp(1, 100),
        ))
          {
            'profile_ref': p['profile_ref'],
            'display_label': p['display_label'],
            'reason_code':
                assignments['${p['profile_ref']}']?['is_control'] == true
                ? 'CONTROL_GROUP'
                : def['experiment_enabled'] == true
                ? 'TREATMENT_VARIANT'
                : 'SOURCE_MEMBER',
            if (def['experiment_enabled'] == true)
              ...assignments['${p['profile_ref']}']!,
          },
      ],
      'warnings': [
        if (people == null) 'Audience membership is not current.',
        'Mock consent source is unavailable. No recipient is cleared for delivery.',
      ],
    });
  }

  JsonObject write(List<String> p, String method, JsonObject b) {
    if (p.isEmpty) {
      final error = validateDefinition(b);
      return error == null ? create(b) : fail(error);
    }
    final c = find(campaigns, 'campaign_ref', p.first);
    if (c == null) return fail('Campaign not found.', 404);
    if (p.length == 1 && method == 'PATCH') {
      if (c['status'] == 'ARCHIVED') {
        return fail('The campaign cannot be edited in this state.', 409);
      }
      if (b['expected_version'] != c['version']) {
        return fail('Refresh the campaign version.', 409);
      }
      final error = validateDefinition(b);
      if (error != null) return fail(error);
      final prior = definitions[p.first]!;
      if ((b['scope_type'] ?? 'BUSINESS') != c['scope_type'] ||
          b['branch_ref'] != c['branch_ref']) {
        return fail('Campaign scope is immutable.', 422);
      }
      final next = definition(b, c['definition_version'] as int);
      final comparablePrior = {...prior}
        ..remove('created_at')
        ..remove('change_reason');
      final comparableNext = {...next}
        ..remove('created_at')
        ..remove('change_reason');
      final changed = jsonEncode(comparablePrior) != jsonEncode(comparableNext);
      final version = (c['definition_version'] as int) + (changed ? 1 : 0);
      definitions[p.first] = definition(b, version);
      c.addAll({
        'name': b['name'],
        'objective': b['objective'] ?? 'GENERAL_MARKETING',
        'scope_type': b['scope_type'] ?? 'BUSINESS',
        'branch_ref': b['scope_type'] == 'BRANCH' ? b['branch_ref'] : null,
        'definition_version': version,
        'version': (c['version'] as int) + 1,
        'status': changed && c['status'] != 'PREPARING' ? 'DRAFT' : c['status'],
        'updated_at': crm.now,
      });
      return response({'campaign': c, 'definition': definitions[p.first]});
    }
    if (p.last == 'archive') {
      if (c['status'] == 'ARCHIVED') return response(c);
      if (b['expected_version'] != c['version']) {
        return fail('Refresh the campaign version.', 409);
      }
      c.addAll({
        'status': 'ARCHIVED',
        'version': (c['version'] as int) + 1,
        'archived_at': crm.now,
      });
      return response(c);
    }
    if (p.last == 'prepare') {
      if (['ARCHIVED', 'PREPARING'].contains(c['status'])) {
        return fail('The campaign cannot be prepared in this state.', 409);
      }
      final def = definitions[p.first]!, people = audience(def);
      if (people == null) {
        return fail('The source or exclusion membership is not current.', 409);
      }
      final preview = jsonObject(previewCampaign(p.first, {})['_data']);
      final ref = id('snapshot');
      for (final old in snapshots[p.first] ?? <JsonObject>[]) {
        if (old['status'] == 'CURRENT') old['status'] = 'SUPERSEDED';
      }
      final snapshot = <String, dynamic>{
        ...preview,
        'snapshot_ref': ref,
        'campaign_definition_version': def['definition_version'],
        'status': 'CURRENT',
        'send_time_consent_recheck_required': true,
        'send_time_frequency_recheck_required': true,
        'send_time_quiet_hours_recheck_required': true,
        'attempt_count': 1,
        'dead_lettered': false,
        'created_at': crm.now,
        'completed_at': crm.now,
      };
      snapshot.remove('sample');
      snapshot.remove('warnings');
      snapshots.putIfAbsent(p.first, () => []).insert(0, snapshot);
      targets[ref] = [
        for (final person in people)
          {
            'target_ref': id('target'),
            'profile_ref': person['profile_ref'],
            'display_label': person['display_label'],
            ...assignment(p.first, def, '${person['profile_ref']}'),
            'currently_actionable': true,
            'service_recovery_active': false,
            'channels': [
              for (final ch in def['channels'] as List)
                {
                  'channel': ch,
                  'snapshot_consent_decision': 'UNKNOWN',
                  'snapshot_consent_eligible': false,
                  'snapshot_eligibility_state': 'INELIGIBLE_CONSENT',
                  'reason_code': 'CONSENT_SOURCE_UNAVAILABLE',
                  'send_time_recheck_required': true,
                },
            ],
          },
      ];
      c.addAll({
        'status': 'PREPARED',
        'current_snapshot_ref': ref,
        'version': (c['version'] as int) + 1,
      });
      return response(snapshot, 202);
    }
    if (p.length == 2 && p.last == 'runs') {
      if (c['status'] != 'PREPARED' ||
          c['current_snapshot_ref'] != b['snapshot_ref']) {
        return fail('A current prepared audience is required.', 409);
      }
      if (c['version'] != b['expected_version']) {
        return fail('Refresh the campaign version.', 409);
      }
      final mode = b['mode'];
      if (!['SEND_NOW', 'SCHEDULED'].contains(mode)) {
        return fail('Choose a valid delivery mode.');
      }
      if (mode == 'SCHEDULED' &&
          DateTime.tryParse('${b['scheduled_for']}')?.isAfter(DateTime.now()) !=
              true) {
        return fail('A future scheduled time is required.');
      }
      if ([
        'template_key',
        'title',
        'body',
      ].every((k) => '${b[k] ?? ''}'.trim().isEmpty)) {
        return fail('Provide a template key or plain text content.');
      }
      if ('${b['title'] ?? ''}'.length > 200 ||
          '${b['body'] ?? ''}'.length > 4000 ||
          '${b['template_key'] ?? ''}'.length > 120 ||
          '${b['locale'] ?? ''}'.length > 16 ||
          RegExp(r'[<>]').hasMatch('${b['title'] ?? ''}${b['body'] ?? ''}')) {
        return fail('Review the content length and use plain text.');
      }
      final ref = id('run');
      final targetRows = targets['${b['snapshot_ref']}'] ?? [];
      final ds = <JsonObject>[
        for (final t in targetRows)
          for (final ch in (t['channels'] as List).where(
            (ch) => ch['snapshot_eligibility_state'] == 'ELIGIBLE_AT_SNAPSHOT',
          ))
            {
              'delivery_ref': id('delivery'),
              'run_ref': ref,
              'campaign_ref': p.first,
              'snapshot_ref': b['snapshot_ref'],
              'target_ref': t['target_ref'],
              'channel': ch['channel'],
              'is_control': false,
              if (t['experiment_variant_key'] != null)
                'variant_key': t['experiment_variant_key'],
              'status': mode == 'SEND_NOW' ? 'SUPPRESSED' : 'PENDING',
              if (mode == 'SEND_NOW') ...<String, dynamic>{
                'suppression_reason': 'CONSENT_SOURCE_UNAVAILABLE',
                'consent_decision_at_handoff': 'UNKNOWN',
                'consent_eligible_at_handoff': false,
                'consent_checked_at': crm.now,
                'policy_checked_at': crm.now,
              },
              'version': 1,
              'created_at': crm.now,
              'updated_at': crm.now,
              'attempt_count': 0,
            },
      ];
      deliveries[ref] = ds;
      for (final d in ds) {
        histories['${d['delivery_ref']}'] = [
          {
            'delivery_ref': d['delivery_ref'],
            'to_status': d['status'],
            'reason_code': d['suppression_reason'],
            'occurred_at': crm.now,
          },
        ];
      }
      final run = <String, dynamic>{
        'run_ref': ref,
        'campaign_ref': p.first,
        'snapshot_ref': b['snapshot_ref'],
        'campaign_definition_version': c['definition_version'],
        'mode': mode,
        'status': mode == 'SEND_NOW' ? 'COMPLETED' : 'SCHEDULED',
        'scheduled_for': b['scheduled_for'] ?? crm.now,
        'total_deliveries': ds.length,
        'handed_off_count': 0,
        'delivered_count': 0,
        'suppressed_count': mode == 'SEND_NOW' ? ds.length : 0,
        'failed_count': 0,
        'version': 1,
        'created_at': crm.now,
        'updated_at': crm.now,
      };
      runs.putIfAbsent(p.first, () => []).insert(0, run);
      return response(run, 201);
    }
    if (p.length == 4 && p[1] == 'runs') {
      final r = find(runs[p.first] ?? [], 'run_ref', p[2]);
      if (r == null) return fail('Run not found.', 404);
      if (r['version'] != b['expected_version']) {
        return fail('Refresh the run version.', 409);
      }
      final status = r['status'], verb = p.last;
      if (verb == 'pause' &&
              !['SCHEDULED', 'QUEUED', 'RUNNING'].contains(status) ||
          verb == 'resume' && status != 'PAUSED' ||
          verb == 'cancel' &&
              [
                'COMPLETED',
                'COMPLETED_WITH_FAILURES',
                'CANCELLED',
                'FAILED',
              ].contains(status)) {
        return fail('This run does not permit that action.', 409);
      }
      if (!['pause', 'resume', 'cancel'].contains(verb)) {
        return fail('Unknown action.', 404);
      }
      r['status'] = verb == 'pause'
          ? 'PAUSED'
          : verb == 'resume'
          ? (DateTime.parse('${r['scheduled_for']}').isAfter(DateTime.now())
                ? 'SCHEDULED'
                : 'QUEUED')
          : 'CANCELLED';
      r['version'] = (r['version'] as int) + 1;
      r['updated_at'] = crm.now;
      if (verb == 'cancel') {
        for (final d in deliveries[p[2]] ?? <JsonObject>[]) {
          if ([
            'PENDING',
            'DEFERRED_QUIET_HOURS',
            'FAILED_TRANSIENT',
          ].contains(d['status'])) {
            d['status'] = 'CANCELLED';
            d['version'] = (d['version'] as int) + 1;
            histories['${d['delivery_ref']}']!.add({
              'delivery_ref': d['delivery_ref'],
              'to_status': 'CANCELLED',
              'occurred_at': crm.now,
            });
          }
        }
      }
      return response(r);
    }
    return fail('Campaign action not found.', 404);
  }
}
