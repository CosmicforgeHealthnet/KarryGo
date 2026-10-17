import 'dart:convert';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_command.dart';
import '../../../core/ui/premium_loading.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../purchasing/ui/purchasing_forms.dart';
import '../../settings/ui/settings_design.dart';
import '../../support/ui/order_support_screen.dart' show supportDate;
import '../state/production_operations_controller.dart';
import 'production_form_screen.dart';

class ProductionReservationScreen extends StatefulWidget {
  const ProductionReservationScreen({
    super.key,
    required this.runtime,
    this.pool,
    this.reservationId,
  });
  final VendorRuntime runtime;
  final JsonObject? pool;
  final String? reservationId;
  @override
  State<ProductionReservationScreen> createState() =>
      _ProductionReservationScreenState();
}

class _ProductionReservationScreenState
    extends State<ProductionReservationScreen> {
  String? id;
  late final c = ProductionOperationsController(
    widget.runtime,
    widget.reservationId == null
        ? '$productionBase/pools'
        : '$productionBase/reservations/{reservationId}',
    parameters: {
      if (widget.reservationId != null) 'reservationId': widget.reservationId!,
    },
  );
  JsonObject? reservation;
  bool reading = false, preparing = false;
  String? action;
  String get bookmarksKey => '${c.recoveryKey}.reservation-references';
  List<String> bookmarks = [];
  @override
  void initState() {
    super.initState();
    id = widget.reservationId;
    c.addListener(changed);
    load();
  }

  void changed() {
    if (mounted) setState(() {});
  }

  @override
  void dispose() {
    c.removeListener(changed);
    c.dispose();
    super.dispose();
  }

  Future<void> remember(String ref) async {
    try {
      final raw = await c.storage.read(bookmarksKey);
      final old = raw == null ? <String>[] : List<String>.from(jsonDecode(raw));
      final next = [ref, ...old.where((r) => r != ref)].take(30).toList();
      await c.storage.write(bookmarksKey, jsonEncode(next));
      if (mounted) setState(() => bookmarks = next);
    } catch (_) {}
  }

  Future<void> load() async {
    await c.load();
    if (!mounted) return;
    try {
      final raw = await c.storage.read(bookmarksKey);
      bookmarks = raw == null ? [] : List<String>.from(jsonDecode(raw));
    } catch (_) {
      bookmarks = [];
    }
    if (id != null && !c.stale) await readReservation(id!);
    if (mounted) setState(() {});
  }

  Future<void> readReservation(String ref) async {
    if (reading) return;
    setState(() {
      reading = true;
      reservation = null;
      c.error = null;
    });
    try {
      final row = jsonObject(
        await c.fetch(
          '$productionBase/reservations/{reservationId}',
          params: {'reservationId': ref},
        ),
      );
      c.validate(row);
      if (mounted) {
        setState(() {
          reservation = row;
          id = ref;
        });
      }
      await remember(ref);
    } catch (_) {
      if (mounted) {
        setState(
          () => c.error =
              'This reservation could not be loaded. Check access and refresh.',
        );
      }
    } finally {
      if (mounted) setState(() => reading = false);
    }
  }

  String? windowError(JsonObject b) =>
      !DateTime.parse(
        b['window_end'],
      ).isAfter(DateTime.parse(b['window_start']))
      ? 'End must be after start.'
      : DateTime.parse(
              b['window_end'],
            ).difference(DateTime.parse(b['window_start'])) >
            const Duration(days: 366)
      ? 'Choose a window of at most 366 days.'
      : null;
  Future<void> create() async {
    final pool = widget.pool!;
    setState(() => preparing = true);
    try {
      final poolRows = c.rows
          .where(
            (p) =>
                p['branch_id'] == pool['branch_id'] &&
                p['capability'] == pool['capability'] &&
                p['status'] == 'ACTIVE',
          )
          .toList();
      final allLines = c.collection(await c.fetch('$productionBase/lines'));
      final fields = <PurchaseField>[
        const PurchaseField(
          'window_start',
          'Window start',
          kind: 'date',
          required: true,
        ),
        const PurchaseField(
          'window_end',
          'Window end',
          kind: 'date',
          required: true,
        ),
        const PurchaseField(
          'priority',
          'Priority',
          kind: 'select',
          required: true,
          initial: 'NORMAL',
          options: {'NORMAL': 'Normal', 'HIGH': 'High', 'URGENT': 'Urgent'},
        ),
        const PurchaseField(
          'hold_for_minutes',
          'Hold duration (minutes, 0 confirms immediately)',
          kind: 'number',
          required: true,
          initial: 0,
        ),
        const PurchaseField('note', 'Note', kind: 'notes'),
        for (final p in poolRows) ...[
          PurchaseField(
            'quantity:${p['id']}',
            '${p['name']} (${dashboardLabel('${p['unit_type']}')})',
            kind: 'number',
            required: true,
            initial: p['id'] == pool['id'] ? 1 : 0,
            max: 1000000,
          ),
          PurchaseField(
            'line:${p['id']}',
            'Production line for ${p['name']}',
            kind: 'select',
            options: {
              for (final l in allLines.where(
                (l) =>
                    l['branch_id'] == pool['branch_id'] &&
                    l['capability'] == pool['capability'] &&
                    l['status'] == 'ACTIVE' &&
                    (l['capacity_pool_id'] == null ||
                        l['capacity_pool_id'] == p['id']),
              ))
                '${l['id']}': '${l['name']}',
            },
          ),
          PurchaseField('family:${p['id']}', 'Item family for ${p['name']}'),
        ],
      ];
      if (!mounted) return;
      final entered = await productionForm(
        context,
        'Reserve capacity',
        fields,
        validate: (b) =>
            windowError(b) ??
            (!poolRows.any((p) => (b['quantity:${p['id']}'] as int) > 0)
                ? 'Allocate a positive quantity to at least one pool.'
                : null),
      );
      if (entered == null || !mounted) return;
      final body = <String, dynamic>{
        'branch_id': pool['branch_id'],
        'capability': pool['capability'],
        'source_type': 'MANUAL',
        'window_start': entered['window_start'],
        'window_end': entered['window_end'],
        'priority': entered['priority'],
        'hold_for_minutes': entered['hold_for_minutes'],
        'note': entered['note'] ?? '',
        'allocations': [
          for (final p in poolRows)
            if ((entered['quantity:${p['id']}'] as int) > 0)
              {
                'capacity_pool_id': p['id'],
                'quantity': entered['quantity:${p['id']}'],
                if (entered['line:${p['id']}'] != null)
                  'production_line_id': entered['line:${p['id']}'],
                if (entered['family:${p['id']}'] != null)
                  'item_family': entered['family:${p['id']}'],
              },
        ],
      };
      final preview = (await widget.runtime.backend.productionApi.request(
        'POST',
        '$productionBase/capacity/preview',
        body: VendorPayload(body),
        command: VendorCommand.create(),
      )).object;
      if (!mounted) return;
      if (preview['available'] != true) {
        await showOrderFeedback(
          context,
          title: 'Capacity unavailable',
          message:
              '${preview['blocking_reason'] ?? 'This window does not have enough capacity.'}',
          success: false,
          warning: true,
        );
        return;
      }
      if (!await confirmSettings(
        context,
        'Reserve this window?',
        'Capacity will be checked again before the reservation is saved.',
      )) {
        return;
      }
      await command('$productionBase/reservations', body);
    } catch (_) {
      if (mounted) {
        setState(
          () => c.error =
              'Capacity references or availability could not be loaded. Try again.',
        );
      }
    } finally {
      if (mounted) setState(() => preparing = false);
    }
  }

  Future<void> command(String route, JsonObject body) async {
    setState(() => action = route);
    final result = await c.save(
      method: 'POST',
      route: route,
      body: body,
      params: {'reservationId': ?id},
    );
    if (!mounted) return;
    if (result != null) {
      id = '${result['id']}';
      await readReservation(id!);
      if (mounted) {
        await showOrderFeedback(
          context,
          title: 'Reservation updated',
          message: 'The capacity reservation has been saved.',
          success: true,
        );
      }
    }
    if (mounted) setState(() => action = null);
  }

  Future<void> retry() async {
    final result = await c.save();
    if (result != null && mounted) {
      id = '${result['id']}';
      await readReservation(id!);
    }
  }

  Future<void> reschedule() async {
    final entered = await productionForm(context, 'Reschedule capacity', [
      PurchaseField(
        'window_start',
        'Window start',
        kind: 'date',
        required: true,
        initial: reservation!['window_start'],
      ),
      PurchaseField(
        'window_end',
        'Window end',
        kind: 'date',
        required: true,
        initial: reservation!['window_end'],
      ),
    ], validate: windowError);
    if (entered != null) {
      await command('$productionBase/reservations/{reservationId}/reschedule', {
        ...entered,
        'expected_version': reservation!['version'],
      });
    }
  }

  Future<void> release({bool partial = false}) async {
    final allocations = c.collection(reservation!['allocations']);
    JsonObject body = {'expected_version': reservation!['version']};
    if (partial) {
      final entered = await productionForm(
        context,
        'Release part of capacity',
        [
          for (final a in allocations)
            PurchaseField(
              '${a['capacity_pool_id']}',
              'Quantity from ${a['capacity_pool_id']}',
              kind: 'number',
              required: true,
              initial: 0,
              max: (a['quantity'] as int) - (a['released_quantity'] as int),
            ),
          const PurchaseField('reason', 'Reason', kind: 'notes'),
        ],
        validate: (b) =>
            allocations.any((a) => (b['${a['capacity_pool_id']}'] as int) > 0)
            ? null
            : 'Choose a positive quantity to release.',
      );
      if (entered == null) return;
      body = {
        ...body,
        'reason': entered['reason'] ?? '',
        'partial': [
          for (final a in allocations)
            if ((entered['${a['capacity_pool_id']}'] as int) > 0)
              {
                'capacity_pool_id': a['capacity_pool_id'],
                'quantity': entered['${a['capacity_pool_id']}'],
              },
        ],
      };
    }
    if (!mounted) return;
    if (!await confirmSettings(
      context,
      'Release capacity?',
      partial
          ? 'Release the selected quantities from this reservation.'
          : 'Release all remaining capacity from this reservation.',
      danger: true,
    )) {
      return;
    }
    await command('$productionBase/reservations/{reservationId}/release', body);
  }

  Future<void> find() async {
    // Offer reservations the vendor can actually reach: ones linked to
    // production jobs plus ones opened on this device. No raw IDs to type.
    final choices = <String, String>{};
    try {
      final jobs = c.collection(
        await c.fetch('$productionBase/jobs', query: {'limit': '100'}),
      );
      for (final job in jobs) {
        final ref = job['reservation_id'];
        if (ref is String && ref.isNotEmpty) {
          choices[ref] = 'Reserved for ${job['title'] ?? 'production job'}';
        }
      }
    } catch (_) {}
    for (final ref in bookmarks) {
      choices.putIfAbsent(ref, () => 'Recently opened reservation');
    }
    if (!mounted) return;
    if (choices.isEmpty) {
      await showOrderFeedback(
        context,
        title: 'No reservations yet',
        message:
            'Create a reservation from a capacity pool, or link one to a production job, and it will appear here.',
        warning: true,
      );
      return;
    }
    final body = await purchaseForm(context, 'Find reservation', [
      PurchaseField(
        'reservation_id',
        'Reservation',
        kind: 'select',
        required: true,
        options: choices,
      ),
    ]);
    if (body != null) await readReservation('${body['reservation_id']}');
  }

  @override
  Widget build(BuildContext context) {
    final r = reservation;
    final active =
        r != null &&
        ['HELD', 'CONFIRMED'].contains(r['status']) &&
        r['consuming_capacity'] == true;
    return Scaffold(
      appBar: AppBar(
        title: const Text('Capacity reservation'),
        actions: [
          IconButton(
            tooltip: 'Refresh',
            onPressed: reading || c.busy ? null : load,
            icon: const Icon(Icons.refresh_rounded),
          ),
        ],
      ),
      body: SafeArea(
        child: ListView(
          padding: const EdgeInsets.fromLTRB(20, 12, 20, 32),
          children: [
            if (c.error != null) SettingsNote(c.error!, warning: true),
            if (c.pending != null) ...[
              const SettingsNote(
                'Reservation outcome is unconfirmed. Retry the original saved request.',
                warning: true,
              ),
              SettingsAction(
                'Retry saved change',
                busy: c.busy,
                onPressed: c.busy ? null : retry,
              ),
            ],
            if (c.loading || reading)
              const PremiumSkeleton(detail: true)
            else if (r != null) ...[
              SettingsCard(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Icon(
                      Icons.bookmark_added_outlined,
                      size: 42,
                      color: Theme.of(context).colorScheme.primary,
                    ),
                    const SizedBox(height: 16),
                    const Text(
                      'Reserved production window',
                      style: TextStyle(
                        fontSize: 26,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                    const SizedBox(height: 12),
                    SettingsStatus(r['status']),
                    SettingsFact(
                      'Business type',
                      dashboardLabel('${r['capability']}'),
                    ),
                    SettingsFact(
                      'Window start',
                      supportDate(r['window_start']),
                    ),
                    SettingsFact('Window end', supportDate(r['window_end'])),
                    if (r['hold_expires_at'] != null)
                      SettingsFact(
                        'Hold expires',
                        supportDate(r['hold_expires_at']),
                      ),
                    SettingsFact(
                      'Priority',
                      dashboardLabel('${r['priority']}'),
                    ),
                    for (final a in c.collection(r['allocations']))
                      SettingsFact(
                        'Pool ${a['capacity_pool_id']}',
                        '${a['quantity']} allocated / ${a['released_quantity']} released',
                      ),
                    SettingsFact('Reference', r['id']),
                  ],
                ),
              ),
              const SizedBox(height: 20),
              if (active && r['status'] == 'HELD')
                SettingsAction(
                  'Confirm reservation',
                  icon: Icons.check_rounded,
                  busy: c.busy && action?.endsWith('/confirm') == true,
                  onPressed:
                      c.allowed(
                        'POST',
                        '$productionBase/reservations/{reservationId}/confirm',
                      )
                      ? () => command(
                          '$productionBase/reservations/{reservationId}/confirm',
                          {'expected_version': r['version']},
                        )
                      : null,
                ),
              if (active) ...[
                const SizedBox(height: 12),
                SettingsAction(
                  'Reschedule',
                  secondary: true,
                  icon: Icons.edit_calendar_outlined,
                  onPressed:
                      c.allowed(
                        'POST',
                        '$productionBase/reservations/{reservationId}/reschedule',
                      )
                      ? reschedule
                      : null,
                ),
                const SizedBox(height: 12),
                SettingsAction(
                  'Release some capacity',
                  secondary: true,
                  onPressed:
                      c.allowed(
                        'POST',
                        '$productionBase/reservations/{reservationId}/release',
                      )
                      ? () => release(partial: true)
                      : null,
                ),
                const SizedBox(height: 12),
                SettingsAction(
                  'Release all capacity',
                  secondary: true,
                  danger: true,
                  onPressed:
                      c.allowed(
                        'POST',
                        '$productionBase/reservations/{reservationId}/release',
                      )
                      ? () => release()
                      : null,
                ),
              ],
            ] else ...[
              if (widget.pool != null) ...[
                SettingsHeading(
                  '${widget.pool!['name']}',
                  subtitle:
                      'Reserve a production window using the available pools.',
                ),
                SettingsAction(
                  'Create reservation',
                  icon: Icons.bookmark_add_outlined,
                  busy: preparing,
                  onPressed:
                      c.allowed('POST', '$productionBase/reservations') &&
                          !preparing
                      ? create
                      : null,
                ),
              ],
              const SizedBox(height: 20),
              SettingsAction(
                'Find a reservation',
                secondary: true,
                icon: Icons.search_rounded,
                onPressed: c.loading || c.busy ? null : find,
              ),
              if (bookmarks.isNotEmpty) ...[
                const SizedBox(height: 24),
                const SettingsHeading('Recently opened'),
                for (final ref in bookmarks)
                  SettingsLink(
                    ref,
                    Icons.history_rounded,
                    onTap: () => readReservation(ref),
                  ),
              ],
            ],
          ],
        ),
      ),
    );
  }
}
