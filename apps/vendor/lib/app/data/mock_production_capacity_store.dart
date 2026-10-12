import 'dart:convert';
import 'package:http/http.dart' as http;
import '../../core/contracts/vendor_contract.dart';

class MockProductionCapacityStore {
  MockProductionCapacityStore(this.pools, this.branches, this.timezones) {
    if (pools.isNotEmpty) {
      final pool = pools.first;
      alerts.add({
        'id': 'mock-production-alert',
        'branch_id': pool['branch_id'],
        'alert_type': 'PRODUCTION_STARTING_SOON',
        'source_type': 'PRODUCTION_JOB',
        'source_ref': 'mock-production-job',
        'status': 'OPEN',
        'payload': {'production_job_id': 'mock-production-job'},
        'detected_at': now,
        'acknowledged_at': null,
        'acknowledged_by': null,
        'resolved_at': null,
        'version': 1,
      });
    }
  }
  final List<JsonObject> pools;
  final List<String> branches;
  final Map<String, String> timezones;
  final lines = <JsonObject>[],
      rules = <JsonObject>[],
      slots = <JsonObject>[],
      templates = <JsonObject>[],
      blackouts = <JsonObject>[],
      alerts = <JsonObject>[];
  final reservations = <String, JsonObject>{};
  int sequence = 0, revision = 1;
  String get now => DateTime.now().toUtc().toIso8601String();
  JsonObject ok(Object? data, [int status = 200]) => {
    '_data': jsonDecode(jsonEncode(data)),
    '_status': status,
  };
  JsonObject fail(String message, [int status = 400]) => {
    '_message': message,
    '_code': 'PRODUCTION_CAPACITY_ERROR',
    '_status': status,
  };
  JsonObject? find(List<JsonObject> rows, Object? id) {
    for (final row in rows) {
      if (row['id'] == id) return row;
    }
    return null;
  }

  bool overlaps(DateTime start, DateTime end, JsonObject r) =>
      DateTime.parse(r['start_at']).isBefore(end) &&
      DateTime.parse(r['end_at']).isAfter(start);
  bool consuming(JsonObject r) =>
      r['status'] == 'CONFIRMED' ||
      r['status'] == 'HELD' &&
          (r['hold_expires_at'] == null ||
              DateTime.parse(
                r['hold_expires_at'],
              ).isAfter(DateTime.now().toUtc()));
  JsonObject view(JsonObject r) => {...r, 'consuming_capacity': consuming(r)};
  DateTime local(DateTime t, String branch) => t.toUtc().add(
    Duration(hours: timezones[branch] == 'Africa/Lagos' ? 1 : 0),
  );
  JsonObject poolLoad(
    JsonObject pool,
    DateTime start,
    DateTime end, {
    String? exclude,
    String? line,
    String? family,
  }) {
    var maximum = pool['default_capacity'] as int;
    String? blockingRule, blocking;
    final clockStart = local(start, pool['branch_id']),
        clockEnd = local(end, pool['branch_id']);
    final date =
        '${clockStart.year}-${clockStart.month.toString().padLeft(2, '0')}-${clockStart.day.toString().padLeft(2, '0')}';
    for (final rule in rules.where(
      (r) => r['capacity_pool_id'] == pool['id'] && r['status'] == 'ACTIVE',
    )) {
      if (rule['production_line_id'] != null &&
              rule['production_line_id'] != line ||
          rule['item_family'] != null &&
              '${rule['item_family']}'.toUpperCase() !=
                  (family ?? '').toUpperCase()) {
        continue;
      }
      final days = rule['days_of_week'] as List? ?? [];
      if (days.isNotEmpty && !days.contains(clockStart.weekday)) continue;
      if ('${rule['effective_start_date'] ?? ''}'.isNotEmpty &&
              date.compareTo(rule['effective_start_date']) < 0 ||
          '${rule['effective_end_date'] ?? ''}'.isNotEmpty &&
              date.compareTo(rule['effective_end_date']) > 0) {
        continue;
      }
      if ('${rule['start_local_time'] ?? ''}'.isNotEmpty) {
        final rs = '${rule['start_local_time']}'.split(':'),
            re = '${rule['end_local_time']}'.split(':');
        final a = int.parse(rs[0]) * 60 + int.parse(rs[1]),
            b = int.parse(re[0]) * 60 + int.parse(re[1]);
        final startMinutes = clockStart.hour * 60 + clockStart.minute,
            endMinutes = clockEnd.hour * 60 + clockEnd.minute;
        final applies = b > a
            ? startMinutes < b && endMinutes > a
            : startMinutes < b || endMinutes > a;
        if (!applies) continue;
      }
      if ((rule['max_quantity'] as int) < maximum) {
        maximum = rule['max_quantity'];
        blockingRule = rule['id'];
      }
    }
    for (final slot in slots.where(
      (r) =>
          r['capacity_pool_id'] == pool['id'] &&
          (r['production_line_id'] == null || r['production_line_id'] == line),
    )) {
      if (['CLOSED', 'BLACKOUT'].contains(slot['status']) &&
          overlaps(start, end, slot)) {
        blocking = 'slot:${slot['id']}';
      }
      if (slot['status'] == 'OPEN' &&
          slot['capacity_override'] != null &&
          !DateTime.parse(slot['start_at']).isAfter(start) &&
          !DateTime.parse(slot['end_at']).isBefore(end) &&
          (slot['capacity_override'] as int) < maximum) {
        maximum = slot['capacity_override'];
      }
    }
    for (final r in blackouts.where(
      (r) =>
          r['status'] == 'ACTIVE' &&
          r['branch_id'] == pool['branch_id'] &&
          (r['capability'] == null || r['capability'] == pool['capability']) &&
          (r['capacity_pool_id'] == null ||
              r['capacity_pool_id'] == pool['id']) &&
          (r['production_line_id'] == null || r['production_line_id'] == line),
    )) {
      if (overlaps(start, end, r)) blocking = r['id'];
    }
    var held = 0, confirmed = 0;
    for (final r in reservations.values.where(
      (r) => r['id'] != exclude && consuming(r),
    )) {
      for (final a in (r['allocations'] as List).map(jsonObject)) {
        if (a['capacity_pool_id'] == pool['id'] &&
            DateTime.parse(a['effective_start']).isBefore(end) &&
            DateTime.parse(a['effective_end']).isAfter(start)) {
          final qty = (a['quantity'] as int) - (a['released_quantity'] as int);
          if (r['status'] == 'HELD') {
            held += qty;
          } else {
            confirmed += qty;
          }
        }
      }
    }
    return {
      'capacity_pool_id': pool['id'],
      'code': pool['code'],
      'unit_type': pool['unit_type'],
      'maximum': maximum,
      'confirmed': confirmed,
      'held': held,
      'remaining': maximum - confirmed - held,
      'blackout_ref': ?blocking,
      'blocking_rule_ref': ?blockingRule,
    };
  }

  JsonObject preview(JsonObject body, {String? exclude}) {
    final start = DateTime.tryParse('${body['window_start']}'),
        end = DateTime.tryParse('${body['window_end']}');
    if (!branches.contains(body['branch_id']) ||
        start == null ||
        end == null ||
        !end.isAfter(start) ||
        end.difference(start) > const Duration(days: 366)) {
      return fail('Choose a valid branch and time window.');
    }
    final out = <JsonObject>[];
    final seen = <String>{};
    for (final a in (body['allocations'] as List? ?? []).map(jsonObject)) {
      final pool = find(pools, a['capacity_pool_id']);
      if (pool == null ||
          pool['branch_id'] != body['branch_id'] ||
          pool['capability'] != body['capability'] ||
          pool['status'] != 'ACTIVE' ||
          a['quantity'] is! int ||
          a['quantity'] <= 0 ||
          a['quantity'] > 1000000 ||
          !seen.add('${pool['id']}')) {
        return fail('Choose matching active pools and valid quantities.');
      }
      final line = find(lines, a['production_line_id']);
      if (a['production_line_id'] != null &&
          (line == null ||
              line['status'] != 'ACTIVE' ||
              line['branch_id'] != pool['branch_id'] ||
              line['capability'] != pool['capability'] ||
              line['capacity_pool_id'] != null &&
                  line['capacity_pool_id'] != pool['id'])) {
        return fail('Choose a matching active production line.');
      }
      final startEffective = start.subtract(
            Duration(
              minutes:
                  line?['setup_buffer_minutes'] ??
                  pool['setup_buffer_minutes'] ??
                  0,
            ),
          ),
          endEffective = end.add(
            Duration(
              minutes:
                  line?['cleanup_buffer_minutes'] ??
                  pool['cleanup_buffer_minutes'] ??
                  0,
            ),
          );
      final load = poolLoad(
        pool,
        startEffective,
        endEffective,
        exclude: exclude,
        line: a['production_line_id'],
        family: a['item_family'] ?? body['source_type'],
      );
      out.add({
        ...load,
        'requested': a['quantity'],
        'sufficient':
            load['blackout_ref'] == null && a['quantity'] <= load['remaining'],
      });
    }
    if (out.isEmpty) return fail('Allocate capacity to at least one pool.');
    final available = out.every((r) => r['sufficient'] == true);
    return ok({
      'window_start': body['window_start'],
      'window_end': body['window_end'],
      'available': available,
      'pools': out,
      'calendar_revision': revision,
      'evaluated_at': now,
      if (!available)
        'blocking_pool_ref': out.firstWhere(
          (r) => r['sufficient'] != true,
        )['capacity_pool_id'],
      if (!available)
        'blocking_reason':
            'This window is closed or exceeds available capacity.',
    });
  }

  List<JsonObject> allocations(JsonObject body) => [
    for (final a in (body['allocations'] as List).map(jsonObject))
      allocation(
        a,
        DateTime.parse(body['window_start']),
        DateTime.parse(body['window_end']),
      ),
  ];
  JsonObject allocation(JsonObject a, DateTime start, DateTime end) {
    final pool = find(pools, a['capacity_pool_id'])!;
    final line = find(lines, a['production_line_id']);
    return {
      'capacity_pool_id': pool['id'],
      if (a['production_line_id'] != null)
        'production_line_id': a['production_line_id'],
      'quantity': a['quantity'],
      'released_quantity': 0,
      'effective_start': start
          .subtract(
            Duration(
              minutes:
                  line?['setup_buffer_minutes'] ?? pool['setup_buffer_minutes'],
            ),
          )
          .toUtc()
          .toIso8601String(),
      'effective_end': end
          .add(
            Duration(
              minutes:
                  line?['cleanup_buffer_minutes'] ??
                  pool['cleanup_buffer_minutes'],
            ),
          )
          .toUtc()
          .toIso8601String(),
    };
  }

  JsonObject? handle(
    http.BaseRequest request,
    JsonObject body,
    List<String> path,
  ) {
    if (path.first == 'capacity') {
      if (path.last == 'preview') return preview(body);
      final query = request.url.queryParameters,
          start = DateTime.tryParse(request.url.queryParameters['start'] ?? ''),
          end = DateTime.tryParse(request.url.queryParameters['end'] ?? '');
      if (start == null ||
          end == null ||
          !end.isAfter(start) ||
          !branches.contains(query['branch_id'])) {
        return fail('Choose a valid utilization window.');
      }
      return ok([
        for (final p in pools.where(
          (p) =>
              p['branch_id'] == query['branch_id'] &&
              p['capability'] == query['capability'] &&
              p['status'] == 'ACTIVE',
        ))
          utilization(p, start, end),
      ]);
    }
    if (path.first == 'reservations') return reservation(request, body, path);
    final collection = switch (path.first) {
      'pools' => pools,
      'lines' => lines,
      'rules' => rules,
      'slots' => slots,
      'slot-templates' => templates,
      'blackouts' => blackouts,
      'alerts' => alerts,
      _ => null,
    };
    if (collection == null) return null;
    if (request.method == 'GET') {
      if (path.length == 1) return ok(collection);
      final r = find(collection, path[1]);
      return r == null ? fail('Record not found.', 404) : ok(r);
    }
    if (path.length == 1 || request.method == 'PATCH') {
      final old = path.length > 1 ? find(collection, path[1]) : null;
      if (path.length > 1 && old == null) return fail('Record not found.', 404);
      if (old != null &&
          (old['version'] != body['expected_version'] ||
              old['status'] == 'ARCHIVED')) {
        return fail('Refresh this record before editing.', 409);
      }
      final data = <String, dynamic>{...body}..remove('expected_version');
      if (data['code'] != null) {
        data['code'] = '${data['code']}'.trim().toUpperCase();
      }
      if (old != null) data['branch_id'] = old['branch_id'];
      if (!branches.contains(data['branch_id'])) {
        return fail('Choose an accessible branch.');
      }
      for (final key in [
        'default_capacity',
        'max_quantity',
        'capacity_override',
      ]) {
        if (data[key] != null &&
            (data[key] is! int || data[key] < 0 || data[key] > 1000000)) {
          return fail('Capacity must be between 0 and 1000000.');
        }
      }
      for (final key in ['setup_buffer_minutes', 'cleanup_buffer_minutes']) {
        if (data[key] != null &&
            (data[key] is! int || data[key] < 0 || data[key] > 480)) {
          return fail('Buffers must be between 0 and 480 minutes.');
        }
      }
      if (data['capacity_pool_id'] != null) {
        final pool = find(pools, data['capacity_pool_id']);
        if (pool == null ||
            pool['status'] != 'ACTIVE' ||
            pool['branch_id'] != data['branch_id'] ||
            pool['capability'] != data['capability']) {
          return fail('Choose a matching active pool.');
        }
      }
      if (data['production_line_id'] != null) {
        final line = find(lines, data['production_line_id']);
        if (line == null ||
            line['status'] != 'ACTIVE' ||
            line['branch_id'] != data['branch_id'] ||
            line['capability'] != data['capability']) {
          return fail('Choose a matching active line.');
        }
      }
      if (path.first == 'slot-templates') {
        if (data['days_of_week'] is! List ||
            (data['days_of_week'] as List).isEmpty ||
            data['generation_horizon_days'] is! int ||
            data['generation_horizon_days'] < 1 ||
            data['generation_horizon_days'] > 366) {
          return fail('Choose weekdays and a valid generation horizon.');
        }
        data['timezone'] = timezones[data['branch_id']];
      }
      if (data['start_at'] != null &&
          !DateTime.parse(
            data['end_at'],
          ).isAfter(DateTime.parse(data['start_at']))) {
        return fail('End must be after start.');
      }
      if (old != null) {
        final mutable = path.first == 'pools'
            ? [
                'name',
                'default_capacity',
                'setup_buffer_minutes',
                'cleanup_buffer_minutes',
              ]
            : path.first == 'slot-templates'
            ? [
                'label',
                'days_of_week',
                'start_local_time',
                'end_local_time',
                'capacity_override',
                'effective_start_date',
                'effective_end_date',
                'timezone',
                'generation_horizon_days',
              ]
            : data.keys.toList();
        for (final key in mutable) {
          old[key] = data[key];
        }
        old['version']++;
        old['updated_at'] = now;
        revision++;
        return ok({'updated': true, 'version': old['version']});
      }
      final id = 'mock-${path.first}-${++sequence}';
      collection.add({
        ...data,
        'id': id,
        'status': path.first == 'slots' ? 'OPEN' : 'ACTIVE',
        'version': 1,
        'created_at': now,
        'updated_at': now,
      });
      revision++;
      return ok({'id': id, 'version': 1}, 201);
    }
    final row = find(collection, path[1]);
    if (row == null) return fail('Record not found.', 404);
    final verb = path.last;
    if (verb == 'generate') return generate(row);
    if (row['version'] != body['expected_version'] ||
        row['status'] == 'ARCHIVED') {
      return fail('Refresh this record before editing.', 409);
    }
    final target = switch (verb) {
      'archive' => 'ARCHIVED',
      'cancel' => 'CANCELLED',
      'open' => 'OPEN',
      'close' => 'CLOSED',
      'blackout' => 'BLACKOUT',
      'acknowledge' => 'ACKNOWLEDGED',
      'resolve' => 'RESOLVED',
      _ => null,
    };
    if (target == null ||
        row['status'] == target ||
        verb == 'cancel' && row['status'] != 'ACTIVE' ||
        verb == 'acknowledge' && row['status'] != 'OPEN' ||
        verb == 'resolve' && row['status'] == 'RESOLVED') {
      return fail('This transition is unavailable.', 409);
    }
    if (target == 'ACKNOWLEDGED') {
      row['acknowledged_at'] = now;
      row['acknowledged_by'] = 'mock-staff-1';
    }
    if (target == 'RESOLVED') row['resolved_at'] = now;
    row['status'] = target;
    row['version']++;
    row['updated_at'] = now;
    if (verb == 'archive') row['archived_at'] = now;
    revision++;
    return ok({'status': target, 'version': row['version']});
  }

  JsonObject utilization(JsonObject pool, DateTime start, DateTime end) {
    final r = poolLoad(pool, start, end);
    return {
      for (final key in [
        'capacity_pool_id',
        'code',
        'unit_type',
        'maximum',
        'confirmed',
        'held',
        'remaining',
      ])
        key: r[key],
      'utilization_basis_points': r['maximum'] == 0
          ? 0
          : ((r['confirmed'] + r['held']) * 10000) ~/ r['maximum'],
    };
  }

  JsonObject reservation(
    http.BaseRequest request,
    JsonObject body,
    List<String> path,
  ) {
    if (path.length == 1) {
      if (body['source_type'] != 'MANUAL') {
        return fail('Only manual reservation fixtures are supported.');
      }
      final check = preview(body);
      if (check['_message'] != null) return check;
      if (jsonObject(check['_data'])['available'] != true) {
        return fail('Capacity unavailable.', 409);
      }
      final id = 'mock-reservation-${++sequence}',
          minutes = body['hold_for_minutes'] as int? ?? 0;
      final row = <String, dynamic>{
        'id': id,
        'branch_id': body['branch_id'],
        'capability': body['capability'],
        'source_type': body['source_type'],
        'source_ref': body['source_ref'],
        'window_start': body['window_start'],
        'window_end': body['window_end'],
        'priority': body['priority'] ?? 'NORMAL',
        'status': minutes > 0 ? 'HELD' : 'CONFIRMED',
        'hold_expires_at': minutes > 0
            ? DateTime.now()
                  .toUtc()
                  .add(Duration(minutes: minutes))
                  .toIso8601String()
            : null,
        'allocations': allocations(body),
        'created_at': now,
        'updated_at': now,
        'version': 1,
      };
      reservations[id] = row;
      revision++;
      return ok(view(row));
    }
    final row = reservations[path[1]];
    if (row == null) return fail('Reservation not found.', 404);
    if (request.method == 'GET') return ok(view(row));
    if (body['expected_version'] != row['version'] || !consuming(row)) {
      return fail('Refresh this active reservation.', 409);
    }
    if (path.last == 'confirm') {
      if (row['status'] != 'HELD') {
        return fail('Only a held reservation can be confirmed.', 409);
      }
      row['status'] = 'CONFIRMED';
      row['hold_expires_at'] = null;
    } else if (path.last == 'release') {
      final partial = (body['partial'] as List? ?? []).map(jsonObject).toList();
      final current = (row['allocations'] as List).map(jsonObject).toList();
      if (partial.isNotEmpty) {
        final seen = <String>{};
        for (final p in partial) {
          final a = findAllocation(current, p['capacity_pool_id']);
          if (a == null ||
              p['quantity'] is! int ||
              p['quantity'] <= 0 ||
              p['quantity'] >
                  (a['quantity'] as int) - (a['released_quantity'] as int) ||
              !seen.add('${p['capacity_pool_id']}')) {
            return fail('Check partial release quantities.');
          }
        }
        for (final p in partial) {
          final a = findAllocation(current, p['capacity_pool_id'])!;
          a['released_quantity'] += p['quantity'];
        }
      } else {
        row['status'] = 'RELEASED';
        for (final a in current) {
          a['released_quantity'] = a['quantity'];
        }
      }
      row['allocations'] = current;
    } else if (path.last == 'reschedule') {
      final old = (row['allocations'] as List).map(jsonObject).toList();
      final wanted = [
        for (final a in old)
          if ((a['quantity'] as int) > (a['released_quantity'] as int))
            {
              'capacity_pool_id': a['capacity_pool_id'],
              'quantity':
                  (a['quantity'] as int) - (a['released_quantity'] as int),
              if (a['production_line_id'] != null)
                'production_line_id': a['production_line_id'],
            },
      ];
      final check = preview({
        ...row,
        ...body,
        'allocations': wanted,
      }, exclude: row['id']);
      if (check['_message'] != null) return check;
      if (jsonObject(check['_data'])['available'] != true) {
        return fail('Capacity unavailable.', 409);
      }
      for (final a in old) {
        final shifted = allocation(
          a,
          DateTime.parse(body['window_start']),
          DateTime.parse(body['window_end']),
        );
        a['effective_start'] = shifted['effective_start'];
        a['effective_end'] = shifted['effective_end'];
      }
      row['window_start'] = body['window_start'];
      row['window_end'] = body['window_end'];
      row['allocations'] = old;
    } else {
      return fail('Unknown reservation action.');
    }
    row['version']++;
    row['updated_at'] = now;
    revision++;
    return ok(view(row));
  }

  JsonObject? findAllocation(List<JsonObject> rows, Object? id) {
    for (final a in rows) {
      if (a['capacity_pool_id'] == id) return a;
    }
    return null;
  }

  JsonObject generate(JsonObject template) {
    if (template['status'] != 'ACTIVE') {
      return fail('Choose an active template.', 404);
    }
    if (!['Africa/Lagos', 'UTC'].contains(template['timezone'])) {
      return fail('This timezone is not simulated in Mock mode.');
    }
    final offset = template['timezone'] == 'Africa/Lagos' ? 1 : 0;
    final localNow = DateTime.now().toUtc().add(Duration(hours: offset));
    final start =
        DateTime.tryParse('${template['effective_start_date'] ?? ''}') ??
        DateTime.utc(localNow.year, localNow.month, localNow.day);
    final end = DateTime.tryParse('${template['effective_end_date'] ?? ''}');
    final a = '${template['start_local_time']}'.split(':'),
        b = '${template['end_local_time']}'.split(':');
    var count = 0;
    for (
      var day = 0;
      day < (template['generation_horizon_days'] as int);
      day++
    ) {
      final date = start.add(Duration(days: day));
      if (end != null && date.isAfter(end)) break;
      if (!(template['days_of_week'] as List).contains(date.weekday)) continue;
      final from = DateTime.utc(
        date.year,
        date.month,
        date.day,
        int.parse(a[0]),
        int.parse(a[1]),
      ).subtract(Duration(hours: offset));
      var to = DateTime.utc(
        date.year,
        date.month,
        date.day,
        int.parse(b[0]),
        int.parse(b[1]),
      ).subtract(Duration(hours: offset));
      if (!to.isAfter(from)) to = to.add(const Duration(days: 1));
      if (slots.any(
        (s) =>
            s['template_id'] == template['id'] &&
            s['start_at'] == from.toIso8601String(),
      )) {
        continue;
      }
      slots.add({
        'id': 'mock-slot-${++sequence}',
        'branch_id': template['branch_id'],
        'capability': template['capability'],
        'capacity_pool_id': template['capacity_pool_id'],
        'production_line_id': template['production_line_id'],
        'template_id': template['id'],
        'label': template['label'],
        'start_at': from.toIso8601String(),
        'end_at': to.toIso8601String(),
        'capacity_override': template['capacity_override'],
        'status': 'OPEN',
        'version': 1,
        'created_at': now,
        'updated_at': now,
      });
      count++;
    }
    revision++;
    return ok({
      'created': count,
      'horizon_days': template['generation_horizon_days'],
      'template_id': template['id'],
    });
  }
}
