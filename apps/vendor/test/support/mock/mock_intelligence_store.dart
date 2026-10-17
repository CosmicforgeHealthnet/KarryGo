import 'dart:convert';
import 'package:http/http.dart' as http;
import 'package:vendor/core/contracts/vendor_contract.dart';
import 'mock_crm_store.dart';

class MockIntelligenceStore {
  MockIntelligenceStore(this.crm);
  final MockCrmStore crm;
  final jobs = <String, JsonObject>{};
  final ticks = <String, int>{};
  final replay = <String, ({String fingerprint, JsonObject result})>{};
  static const labels = {
    'RFM_V1': 'RFM (recency, frequency, monetary) V1',
    'CHURN_V1': 'Churn risk heuristic V1',
  };
  String identity(String model, String? branch) => '$model|${branch ?? ''}';
  int count(String? branch) =>
      branch == null || branch == crm.business.branches.firstOrNull?['id']
      ? crm.profiles
            .where(
              (p) =>
                  p['status'] == 'ACTIVE' &&
                  p['privacy_state'] == 'ACTIVE' &&
                  p['merge_state'] != 'MERGED_SOURCE',
            )
            .length
      : 0;
  JsonObject availability(String model, String? branch) {
    final key = identity(model, branch), job = jobs[key];
    if (job != null && ['QUEUED', 'RUNNING'].contains(job['status'])) {
      final tick = ticks.update(key, (v) => v + 1, ifAbsent: () => 1);
      job['status'] = tick < 2 ? 'RUNNING' : 'COMPLETED';
      if (tick >= 2) job['completed_at'] = crm.now;
    }
    return {
      'model_key': model,
      'label': labels[model],
      'status': job == null
          ? 'NOT_AVAILABLE'
          : job['status'] == 'COMPLETED'
          ? 'CURRENT'
          : job['status'],
      'profile_count': job == null ? 0 : count(branch),
      'available_count': 0,
      'unavailable_count': job == null ? 0 : count(branch),
      if (job != null) 'model_version': 1,
      if (job != null) 'generation': job['generation'],
      if (job != null) 'source_as_of': job['source_as_of'],
      if (job?['completed_at'] != null) 'computed_at': job!['completed_at'],
    };
  }

  JsonObject handle(
    http.BaseRequest request,
    JsonObject body,
    List<String> path,
  ) {
    final branch = request.url.queryParameters['branch_ref'];
    if (branch != null && !crm.validBranch(branch)) {
      return crm.fail('This branch is unavailable.', 403);
    }
    if (request.method == 'GET') {
      if (path.last == 'overview') {
        return crm.ok({
          'scope': branch == null ? 'BUSINESS' : 'BRANCH',
          'branch_ref': ?branch,
          'models': [
            for (final model in labels.keys) availability(model, branch),
          ],
        });
      }
      final model = path.last == 'rfm'
          ? 'RFM_V1'
          : path.last == 'churn'
          ? 'CHURN_V1'
          : null;
      if (model == null) return crm.fail('Insights route not found.', 404);
      final state = availability(model, branch);
      return crm.ok({
        'availability': state,
        if (model == 'RFM_V1') ...{
          'composite_distribution': [],
          'recency_distribution': [],
          'frequency_distribution': [],
          'monetary_distribution': [],
        },
        if (model == 'CHURN_V1') 'band_distribution': [],
        'unavailable_reasons': state['unavailable_count'] == 0
            ? []
            : [
                {
                  'key': 'SOURCE_NOT_AVAILABLE',
                  'count': state['unavailable_count'],
                },
              ],
      });
    }
    if (request.method != 'POST' || path.last != 'rebuild') {
      return crm.fail('Insights action unavailable.', 404);
    }
    final key =
        request.headers['Idempotency-Key'] ??
        request.headers['idempotency-key'];
    if (key == null || key.isEmpty) {
      return crm.fail('A submission key is required.');
    }
    final fingerprint = jsonEncode(body), saved = replay[key];
    if (saved != null) {
      return saved.fingerprint == fingerprint
          ? saved.result
          : crm.fail('Rebuild settings changed.', 409);
    }
    final model = body['model_key'], target = body['branch_ref'] as String?;
    if (!labels.containsKey(model) ||
        !['BUSINESS', 'BRANCH'].contains(body['scope_type']) ||
        body['scope_type'] == 'BRANCH' &&
            (target == null || !crm.validBranch(target)) ||
        body['scope_type'] == 'BUSINESS' && target != null) {
      return crm.fail('Choose a supported model and scope.');
    }
    final id = identity(model, target), previous = jobs[id];
    final job =
        previous != null && ['QUEUED', 'RUNNING'].contains(previous['status'])
        ? previous
        : <String, dynamic>{
            'evaluation_ref':
                'mock-evaluation-${jobs.length + 1}-${(previous?['generation'] as int? ?? 0) + 1}',
            'model_key': model,
            'model_version': 1,
            'scope': body['scope_type'],
            'branch_ref': ?target,
            'generation': (previous?['generation'] as int? ?? 0) + 1,
            'status': 'QUEUED',
            'source_as_of': crm.now,
            'requested_at': crm.now,
            'profile_count': 0,
            'available_count': 0,
            'unavailable_count': 0,
          };
    jobs[id] = job;
    ticks[id] = 0;
    final result = jsonObject(jsonDecode(jsonEncode(crm.ok(job, 202))));
    replay[key] = (fingerprint: fingerprint, result: result);
    return result;
  }
}
