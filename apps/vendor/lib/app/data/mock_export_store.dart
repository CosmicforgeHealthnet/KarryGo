import 'dart:convert';
import 'package:crypto/crypto.dart';
import 'package:http/http.dart' as http;
import '../../core/contracts/vendor_contract.dart';
import 'mock_crm_store.dart';

const mockExportFields = <String, List<String>>{
  "CUSTOMER_SUMMARY": [
    "profile_ref",
    "subject_type",
    "status",
    "acquisition_source_type",
    "created_at",
    "commercial_projection_state",
    "completed_order_count",
    "net_spend_minor",
    "commercial_currency",
    "last_completed_order_at",
    "rfm_state",
    "rfm_composite_score",
    "churn_state",
    "churn_risk_score",
    "churn_risk_band",
  ],
  "SEGMENT_MEMBERS": [
    "segment_ref",
    "generation",
    "profile_ref",
    "membership_source",
    "profile_status",
    "commercial_projection_state",
  ],
  "CORPORATE_CUSTOMERS": [
    "corporate_ref",
    "corporate_customer_code",
    "organization_name",
    "organization_type",
    "status",
    "scope_mode",
    "account_manager_staff_ref",
    "relationship_started_at",
    "location_count",
    "cost_center_count",
    "contact_count",
  ],
  "LOYALTY_SUMMARY": [
    "profile_ref",
    "account_state",
    "available_points",
    "lifetime_credited_points",
    "lifetime_debited_points",
    "next_expiry_at",
    "next_expiry_points",
  ],
  "CAMPAIGN_PERFORMANCE_SUMMARY": [
    "campaign_ref",
    "campaign_version",
    "run_ref",
    "run_status",
    "scheduled_for",
    "completed_at",
    "target_count",
    "handed_off_count",
    "delivered_count",
    "suppressed_count",
    "failed_count",
  ],
  "INTELLIGENCE_SCORES": [
    "profile_ref",
    "scope",
    "branch_ref",
    "model_key",
    "model_version",
    "state",
    "recency_score",
    "frequency_score",
    "monetary_score",
    "composite_score",
    "risk_score",
    "risk_band",
    "reason_codes",
    "source_as_of",
    "computed_at",
  ],
};

class MockExportStore {
  final downloadedKeys = <String>{};
  MockExportStore(this.crm);
  final MockCrmStore crm;
  final jobs = <JsonObject>[];
  final files = <String, List<int>>{};
  final ticks = <String, int>{};
  final commands = <String, ({String fingerprint, JsonObject result})>{};
  int sequence = 0;
  JsonObject copy(JsonObject row) => jsonObject(jsonDecode(jsonEncode(row)));
  List<JsonObject> source(JsonObject job) {
    final filter = jsonObject(job['filter']);
    var profiles = crm.profiles.where(crm.actionable).toList();
    if (job['scope'] == 'BRANCH' &&
        job['branch_ref'] != crm.business.branches.firstOrNull?['id']) {
      profiles = [];
    }
    final statuses = filter['statuses'] as List? ?? [];
    if (statuses.isNotEmpty) {
      profiles = profiles.where((p) => statuses.contains(p['status'])).toList();
    }
    switch (job['export_type']) {
      case 'CUSTOMER_SUMMARY':
        return [
              for (final p in profiles)
                {
                  ...p,
                  'commercial_projection_state': 'NOT_AVAILABLE',
                  'rfm_state': 'NOT_AVAILABLE',
                  'churn_state': 'NOT_AVAILABLE',
                },
            ]
            .where(
              (r) =>
                  (filter['risk_bands'] as List? ?? []).isEmpty &&
                  ((filter['score_states'] as List? ?? []).isEmpty ||
                      (filter['score_states'] as List).contains(
                        'NOT_AVAILABLE',
                      )),
            )
            .toList();
      case 'SEGMENT_MEMBERS':
        return [
          for (final p in profiles)
            if (crm.members[filter['segment_ref']]?.contains(
                  p['profile_ref'],
                ) ==
                true)
              {
                'segment_ref': filter['segment_ref'],
                'profile_ref': p['profile_ref'],
                'membership_source': 'STATIC',
                'profile_status': p['status'],
                'commercial_projection_state': 'NOT_AVAILABLE',
              },
        ];
      case 'CORPORATE_CUSTOMERS':
        return crm
            .bucket('corporate-customers')
            .where(
              (r) =>
                  (statuses.isEmpty || statuses.contains(r['status'])) &&
                  (job['scope'] == 'BUSINESS' ||
                      (r['scope_mode'] == 'BRANCH_SET' &&
                          (r['branch_refs'] as List? ?? []).contains(
                            job['branch_ref'],
                          ))),
            )
            .toList();
      case 'LOYALTY_SUMMARY':
        return [
          for (final p in profiles)
            if (crm.balances.containsKey(p['profile_ref']))
              {
                'profile_ref': p['profile_ref'],
                'available_points': crm.balances[p['profile_ref']],
              },
        ];
      case 'CAMPAIGN_PERFORMANCE_SUMMARY':
        return [
          for (final group in crm.campaigns.runs.values)
            for (final r in group)
              if ((filter['campaign_ref'] == null ||
                      filter['campaign_ref'] == r['campaign_ref']) &&
                  (statuses.isEmpty || statuses.contains(r['status'])) &&
                  (job['scope'] == 'BUSINESS' ||
                      r['branch_ref'] == job['branch_ref']))
                {...r, 'run_status': r['status']},
        ];
      default:
        return [];
    }
  }

  void progress(JsonObject job) {
    if (!['QUEUED', 'RUNNING'].contains(job['status'])) return;
    final tick = (ticks[job['export_ref']] ?? 0) + 1;
    ticks[job['export_ref']] = tick;
    job['status'] = 'RUNNING';
    job['started_at'] ??= crm.now;
    if (tick < 2) return;
    final data = source(job), fields = List<String>.from(job['fields']);
    if (data.length > job['row_limit']) {
      job['status'] = 'FAILED';
      job['error_code'] = 'CRM_EXPORT_TOO_LARGE';
      job['failed_at'] = crm.now;
      job['version']++;
      return;
    }
    final rows = [
      for (final r in data) {for (final f in fields) f: r[f]},
    ];
    String cell(Object? value) {
      var s = value?.toString() ?? '';
      if (RegExp(r'^[=+@\-\t\r]').hasMatch(s)) s = "'$s";
      return '"${s.replaceAll('"', '""')}"';
    }

    final content = job['format'] == 'CSV'
        ? ('${fields.map(cell).join(',')}\r\n${rows.map((r) => fields.map((f) => cell(r[f])).join(',')).join('\r\n')}${rows.isEmpty ? '' : '\r\n'}')
        : rows.map(jsonEncode).join('\n');
    final bytes = utf8.encode(content);
    files[job['export_ref']] = bytes;
    job.addAll({
      'status': 'READY',
      'row_count': rows.length,
      'size_bytes': bytes.length,
      'checksum_sha256': sha256.convert(bytes).toString(),
      'completed_at': crm.now,
      'expires_at': DateTime.now()
          .toUtc()
          .add(const Duration(days: 1))
          .toIso8601String(),
      'version': job['version'] + 1,
    });
  }

  JsonObject handle(
    http.BaseRequest request,
    JsonObject body,
    List<String> path,
  ) {
    final id = path.length > 1 ? path[1] : null;
    if (request.method == 'GET') {
      for (final job in jobs) {
        if (job['status'] == 'READY' &&
            DateTime.parse(job['expires_at']).isBefore(DateTime.now())) {
          job['status'] = 'EXPIRED';
          files.remove(job['export_ref']);
          job['version']++;
        }
        progress(job);
      }
      if (id != null) {
        final row = jobs.where((j) => j['export_ref'] == id).firstOrNull;
        return row == null ? crm.fail('Export not found.', 404) : crm.ok(row);
      }
      final q = request.url.queryParameters,
          rows = jobs.reversed
              .where(
                (j) =>
                    (q['status'] == null || q['status'] == j['status']) &&
                    (q['export_type'] == null ||
                        q['export_type'] == j['export_type']),
              )
              .toList();
      final limit = (int.tryParse(q['limit'] ?? '50') ?? 50).clamp(1, 200),
          offset = (int.tryParse(q['offset'] ?? '0') ?? 0).clamp(
            0,
            rows.length,
          );
      return crm.ok({
        'exports': rows.skip(offset).take(limit).toList(),
        'limit': limit,
        'offset': offset,
        'has_more': offset + limit < rows.length,
      });
    }
    final purpose = id == null ? 'crm.export.create' : 'crm.export.download';
    final file = id != null && path.last == 'file';
    final key =
        request.headers['Idempotency-Key'] ??
        request.headers['idempotency-key'];
    final semantic = {...body}..remove('step_up_grant_id');
    final fingerprint = jsonEncode({
      'path': request.url.path,
      'body': semantic,
    });
    if (key == null || key.isEmpty) {
      return crm.fail('A submission key is required.', 400);
    }
    if (!file) {
      final prior = commands[key];
      if (prior != null) {
        if (prior.fingerprint != fingerprint) {
          return crm.fail('Export submission settings changed.', 409);
        }
        if (id == null &&
            !crm.business.consumeExportGrant(
              '${body['step_up_grant_id'] ?? ''}',
              purpose,
            )) {
          return crm.fail('Additional verification is required.', 403);
        }
        return copy(prior.result);
      }
    }
    if (id == null) {
      if (body['row_limit'] != null &&
          (body['row_limit'] is! int ||
              body['row_limit'] < 1 ||
              body['row_limit'] > 250000)) {
        return crm.fail('Choose a row limit from 1 to 250,000.');
      }
      if (!mockExportFields.containsKey(body['export_type']) ||
          !['CSV', 'JSONL'].contains(body['format']) ||
          !['BUSINESS', 'BRANCH'].contains(body['scope_type'])) {
        return crm.fail('Choose a supported export dataset, format and scope.');
      }
      if (body['scope_type'] == 'BRANCH' &&
          (body['branch_ref'] == null ||
              !crm.validBranch(body['branch_ref']))) {
        return crm.fail('Choose an available branch.');
      }
      final filter = jsonObject(body['filter'] ?? {});
      if (body['export_type'] == 'SEGMENT_MEMBERS' &&
          !crm
              .bucket('segments')
              .any(
                (r) =>
                    r['segment_ref'] == filter['segment_ref'] &&
                    r['status'] != 'ARCHIVED',
              )) {
        return crm.fail('Choose an available segment.');
      }
      if (!crm.business.consumeExportGrant(
        '${body['step_up_grant_id'] ?? ''}',
        purpose,
      )) {
        return crm.fail('Additional verification is required.', 403);
      }
      final ref = 'mock-export-${++sequence}',
          row = <String, dynamic>{
            'export_ref': ref,
            'export_type': body['export_type'],
            'format': body['format'],
            'status': 'QUEUED',
            'scope': body['scope_type'],
            if (body['scope_type'] == 'BRANCH')
              'branch_ref': body['branch_ref'],
            'requested_by_staff_ref': crm.settings.staff.first['id'],
            'filter': filter,
            'field_set_version': 1,
            'fields': mockExportFields[body['export_type']],
            'row_limit': body['row_limit'] ?? 100000,
            'row_count': 0,
            'requested_at': crm.now,
            'version': 1,
          };
      jobs.add(row);
      final result = crm.ok(row, 202);
      commands[key] = (fingerprint: fingerprint, result: copy(result));
      return result;
    }
    final job = jobs.where((j) => j['export_ref'] == id).firstOrNull;
    if (job == null) return crm.fail('Export not found.', 404);
    if (path.last == 'cancel') {
      if (body['expected_version'] != job['version'] ||
          ['FAILED', 'EXPIRED'].contains(job['status'])) {
        return crm.fail('This export changed. Refresh before cancelling.', 409);
      }
      files.remove(id);
      job.addAll({
        'status': 'CANCELLED',
        'cancelled_at': crm.now,
        'version': job['version'] + 1,
      });
      final result = crm.ok(job);
      commands[key] = (fingerprint: fingerprint, result: copy(result));
      return result;
    }
    if (file) {
      if (downloadedKeys.contains(key)) {
        return crm.fail('Verify again before another download.', 409);
      }
      if (job['status'] != 'READY' ||
          files[id] == null ||
          DateTime.parse(job['expires_at']).isBefore(DateTime.now())) {
        return crm.fail('This export is not ready or has expired.', 409);
      }
      if (!crm.business.consumeExportGrant(
        '${body['step_up_grant_id'] ?? ''}',
        purpose,
      )) {
        return crm.fail('Additional verification is required.', 403);
      }
      downloadedKeys.add(key);
      return {
        '_bytes': files[id],
        '_mime': job['format'] == 'CSV' ? 'text/csv' : 'application/x-ndjson',
        '_status': 200,
      };
    }
    return crm.fail('Export action unavailable.', 404);
  }
}
