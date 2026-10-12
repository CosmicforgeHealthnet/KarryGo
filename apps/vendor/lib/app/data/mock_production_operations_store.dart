import 'mock_production_capacity_store.dart';
import 'dart:convert';
import 'package:http/http.dart' as http;
import '../../core/contracts/vendor_contract.dart';
import '../../features/production/state/production_transitions.dart';

class MockProductionOperationsStore {
  MockProductionOperationsStore(
    this.branches, {
    required bool empty,
    this.timezones = const {},
  }) {
    if (!empty && branches.isNotEmpty) {
      jobs.add({
        'id': 'mock-production-job',
        'branch_id': branches.first,
        'capability': 'RESTAURANT',
        'source_type': 'MANUAL',
        'source_ref': null,
        'title': 'Lunch preparation',
        'status': 'PLANNED',
        'priority': 'NORMAL',
        'planned_quantity': 24,
        'produced_quantity': 0,
        'waste_quantity': 0,
        'blocked_reason': '',
        'version': 1,
        'created_at': now,
        'updated_at': now,
        'promised_ready_at': null,
      });
      steps['mock-production-job'] = [
        {
          'id': 'mock-production-step',
          'name': 'Prepare ingredients',
          'sequence_no': 0,
          'phase': 0,
          'required': true,
          'status': 'PENDING',
          'blocked_reason': '',
          'version': 1,
        },
      ];
      pools.add({
        'id': 'mock-capacity-pool',
        'branch_id': branches.first,
        'capability': 'RESTAURANT',
        'name': 'Kitchen capacity',
        'code': 'KITCHEN',
        'unit_type': 'MEAL',
        'default_capacity': 80,
        'setup_buffer_minutes': 0,
        'cleanup_buffer_minutes': 0,
        'scope_type': 'BRANCH',
        'scope_ref': null,
        'status': 'ACTIVE',
        'version': 1,
        'created_at': now,
        'updated_at': now,
      });
    }
  }
  final List<String> branches;
  final Map<String, String> timezones;
  late final capacity = MockProductionCapacityStore(pools, branches, timezones);
  bool hasJob(String id, String branch) => jobs.any(
    (j) =>
        j['id'] == id &&
        j['branch_id'] == branch &&
        !['CANCELLED', 'COMPLETED'].contains(j['status']),
  );
  final jobs = <JsonObject>[], pools = <JsonObject>[];
  final steps = <String, List<JsonObject>>{},
      events = <String, List<JsonObject>>{};
  final replay = <String, ({String fingerprint, JsonObject result})>{};
  int sequence = 0;
  String get now => DateTime.now().toUtc().toIso8601String();
  JsonObject ok(Object? data, [int status = 200]) => {
    '_data': jsonDecode(jsonEncode(data)),
    '_status': status,
  };
  JsonObject fail(String message, [int status = 400]) => {
    '_message': message,
    '_code': 'PRODUCTION_ERROR',
    '_status': status,
  };
  JsonObject? find(List<JsonObject> rows, String id) {
    for (final r in rows) {
      if (r['id'] == id) return r;
    }
    return null;
  }

  JsonObject? handle(http.BaseRequest request, JsonObject body) {
    const root = '/api/v1/vendor/production/';
    if (!request.url.path.startsWith(root)) return null;
    final path = request.url.path.substring(root.length).split('/');
    final write = request.method != 'GET';
    final key =
        request.headers['Idempotency-Key'] ??
        request.headers['idempotency-key'] ??
        '';
    final fingerprint = jsonEncode([
      request.method,
      request.url.path,
      body,
      request.headers['If-Match'] ?? request.headers['if-match'],
    ]);
    if (write) {
      if (key.isEmpty) return fail('An idempotency key is required.');
      final old = replay[key];
      if (old != null) {
        return old.fingerprint == fingerprint
            ? old.result
            : fail('This key belongs to another request.', 409);
      }
    }
    JsonObject result;
    final capacityResult = capacity.handle(request, body, path);
    if (capacityResult != null) {
      result = capacityResult;
    } else if (path.first == 'jobs') {
      if (path.length == 1) {
        if (!write) {
          final status = request.url.queryParameters['status'] ?? 'ACTIVE';
          final rows = jobs
              .where(
                (j) =>
                    status == 'ALL' ||
                    status == 'ACTIVE' &&
                        !['COMPLETED', 'CANCELLED'].contains(j['status']) ||
                    j['status'] == status,
              )
              .toList();
          rows.sort((a, b) {
            final ap = a['promised_ready_at'], bp = b['promised_ready_at'];
            if (ap != bp) {
              if (ap == null) return 1;
              if (bp == null) return -1;
              return '$ap'.compareTo('$bp');
            }
            const rank = {'URGENT': 0, 'HIGH': 1, 'NORMAL': 2};
            return rank[a['priority']]!.compareTo(rank[b['priority']]!);
          });
          result = ok([
            for (final j in rows)
              {
                ...j,
                'step_count': (steps[j['id']] ?? []).length,
                'completed_step_count': (steps[j['id']] ?? [])
                    .where(
                      (s) => ['COMPLETED', 'SKIPPED'].contains(s['status']),
                    )
                    .length,
                'blocking_step_count': (steps[j['id']] ?? [])
                    .where((s) => s['status'] == 'BLOCKED')
                    .length,
              },
          ]);
        } else {
          if (!branches.contains(body['branch_id']) ||
              '${body['title'] ?? ''}'.trim().length < 2 ||
              body['planned_quantity'] is! int ||
              body['planned_quantity'] < 1 ||
              body['source_type'] != 'MANUAL') {
            return fail('Check the job details.');
          }
          final id = 'mock-job-${++sequence}';
          jobs.add({
            ...body,
            'id': id,
            'status': 'PLANNED',
            'produced_quantity': 0,
            'waste_quantity': 0,
            'blocked_reason': '',
            'version': 1,
            'created_at': now,
            'updated_at': now,
          });
          events[id] = [
            {
              'id': 'event-${++sequence}',
              'event_type': 'CREATED',
              'to_status': 'PLANNED',
              'note': '',
              'recorded_at': now,
            },
          ];
          result = ok({'id': id}, 201);
        }
      } else {
        final job = find(jobs, path[1]);
        if (job == null) return fail('Job not found.', 404);
        if (!write) {
          result = ok(
            path.length == 2
                ? job
                : path.last == 'steps'
                ? steps[job['id']] ?? []
                : events[job['id']] ?? [],
          );
        } else {
          final expected = int.tryParse(
            request.headers['If-Match'] ?? request.headers['if-match'] ?? '',
          );
          if (expected != job['version']) {
            return fail('Refresh this job before editing.', 409);
          }
          if (path.length == 3 && path.last == 'steps') {
            if ('${body['name'] ?? ''}'.trim().isEmpty ||
                body['expected_job_version'] != job['version']) {
              return fail('Check the station step details.');
            }
            final id = 'mock-step-${++sequence}';
            (steps[job['id']] ??= []).add({
              ...body,
              'id': id,
              'status': 'PENDING',
              'blocked_reason': '',
              'version': 1,
            });
            result = ok({'id': id}, 201);
          } else if (path.length == 5 && path[2] == 'steps') {
            final step = find(steps[job['id']] ?? [], path[3]);
            final target = path[4];
            if (step == null) return fail('Step not found.', 404);
            if (body['expected_version'] != step['version'] ||
                !(stationTransitions[step['status']] ?? []).contains(target)) {
              return fail('Refresh this station step.', 409);
            }
            if (target == 'READY' &&
                (steps[job['id']] ?? []).any(
                  (s) =>
                      s['required'] == true &&
                      (s['sequence_no'] as int) <
                          (step['sequence_no'] as int) &&
                      !['COMPLETED', 'SKIPPED'].contains(s['status']),
                )) {
              return fail('Complete prior required steps first.', 409);
            }
            if (target == 'BLOCKED' &&
                '${body['blocked_reason'] ?? ''}'.trim().isEmpty) {
              return fail('A blocked reason is required.');
            }
            step['status'] = target;
            step['version']++;
            step['blocked_reason'] = target == 'BLOCKED'
                ? body['blocked_reason']
                : '';
            if (['BLOCKED', 'IN_PROGRESS', 'READY'].contains(target) &&
                !['COMPLETED', 'CANCELLED'].contains(job['status'])) {
              job['status'] = target == 'BLOCKED' ? 'BLOCKED' : 'IN_PROGRESS';
              job['version']++;
              job['blocked_reason'] = step['blocked_reason'];
            }
            result = ok({'status': target});
          } else if (path.last == 'delay') {
            const reasons = [
              'STAFF_SHORTAGE',
              'EQUIPMENT_FAILURE',
              'INGREDIENT_SHORTAGE',
              'QUALITY_REWORK',
              'CAPACITY_CONFLICT',
              'LOGISTICS_CONSTRAINT',
              'OTHER',
            ];
            final start = DateTime.tryParse('${body['new_window_start']}'),
                end = DateTime.tryParse('${body['new_window_end']}');
            if (!reasons.contains(body['reason_code']) ||
                '${body['note'] ?? ''}'.length > 280 ||
                start == null ||
                end == null ||
                !end.isAfter(start)) {
              return fail('Check the new ready window and delay reason.');
            }
            job['expected_ready_window_start'] = body['new_window_start'];
            job['expected_ready_window_end'] = body['new_window_end'];
            job['version']++;
            (events[job['id']] ??= []).add({
              'id': 'event-${++sequence}',
              'event_type': 'DELAY_REPORTED',
              'note': body['note'] ?? '',
              'recorded_at': now,
            });
            result = ok({'status': 'RECORDED'});
          } else {
            final target = switch (path.last) {
              'start' => 'IN_PROGRESS',
              'ready' => 'READY',
              'complete' => 'COMPLETED',
              _ => path.last,
            };
            if (!(jobTransitions[job['status']] ?? []).contains(target)) {
              return fail('This job transition is unavailable.', 409);
            }
            if (target == 'BLOCKED' &&
                '${body['blocked_reason'] ?? ''}'.trim().isEmpty) {
              return fail('A blocked reason is required.');
            }
            if (target == 'READY') {
              if ((steps[job['id']] ?? []).any(
                (step) =>
                    step['required'] == true &&
                    !['COMPLETED', 'SKIPPED'].contains(step['status']),
              )) {
                return fail('Complete required station steps first.', 409);
              }
              if (body['packaging_profile'] is! Map ||
                  (body['packaging_profile'] as Map).isEmpty ||
                  body['packaging_evidence'] is! Map ||
                  (body['packaging_evidence'] as Map).isEmpty) {
                return fail(
                  'Record packaging profile and evidence before handoff.',
                );
              }
            }
            if (['READY', 'COMPLETED'].contains(target)) {
              final produced = body['produced_quantity'],
                  waste = body['waste_quantity'];
              if (produced is! int ||
                  waste is! int ||
                  produced < 0 ||
                  waste < 0 ||
                  produced + waste > job['planned_quantity']) {
                return fail('Check produced and waste quantities.');
              }
              job['produced_quantity'] = produced;
              job['waste_quantity'] = waste;
            }
            if (target == 'READY') {
              job['ready_evidence_id'] = 'mock-ready-${++sequence}';
              job['ready_at'] = now;
            }
            if (target == 'COMPLETED') job['completed_at'] = now;
            final previous = job['status'];
            job['status'] = target;
            job['version']++;
            job['blocked_reason'] = target == 'BLOCKED'
                ? body['blocked_reason']
                : '';
            (events[job['id']] ??= []).add({
              'id': 'event-${++sequence}',
              'event_type': 'STATUS_CHANGED',
              'from_status': previous,
              'to_status': target,
              'note': body['note'] ?? '',
              'recorded_at': now,
            });
            result = ok({'status': target});
          }
        }
      }
    } else {
      return fail('This production workflow is not yet simulated.', 404);
    }
    if (write && result['_message'] == null) {
      replay[key] = (fingerprint: fingerprint, result: result);
    }
    return result;
  }
}
