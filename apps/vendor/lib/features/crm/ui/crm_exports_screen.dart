import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/ui/premium_loading.dart';
import '../../onboarding/ui/setup_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../orders/ui/order_form_sheet.dart';
import '../../production/ui/production_form_screen.dart';
import '../../purchasing/ui/purchasing_forms.dart';
import '../../settings/state/settings_controller.dart';
import '../../settings/ui/settings_design.dart';
import '../../settings/ui/settings_verification_screen.dart';
import '../../support/ui/order_support_screen.dart' show supportDate;
import '../state/crm_export_controller.dart';
import '../state/crm_workspace_controller.dart';
import 'crm_editors.dart';
import 'export_file_save.dart';

class CrmExportsScreen extends StatefulWidget {
  const CrmExportsScreen({super.key, required this.runtime, this.id});
  final VendorRuntime runtime;
  final String? id;
  @override
  State<CrmExportsScreen> createState() => _CrmExportsScreenState();
}

class _CrmExportsScreenState extends State<CrmExportsScreen> {
  late final c = CrmExportController(widget.runtime, id: widget.id);
  bool acting = false;
  String? action, status;
  @override
  void initState() {
    super.initState();
    load();
  }

  Future<void> load({bool more = false}) async {
    c.readQuery = {
      'limit': '50',
      'offset': more ? '${c.rows.length}' : '0',
      'status': ?status,
    };
    await c.load(append: more);
  }

  Future<void> message(String title, String text, {bool success = false}) =>
      showOrderFeedback(
        context,
        title: title,
        message: text,
        success: success,
        warning: !success,
      );
  Future<String?> verify(String purpose, String title) =>
      Navigator.push<String>(
        context,
        setupRoute<String>(
          SettingsVerificationScreen(
            runtime: widget.runtime,
            purpose: purpose,
            title: title,
            actionLabel: 'Continue',
          ),
        ),
      );
  Future<List<JsonObject>> choices(String route, String wrapper) async {
    final rows = <JsonObject>[];
    while (true) {
      final value = jsonObject(
        await c.fetch(
          route,
          query: {'limit': '100', 'offset': '${rows.length}'},
        ),
      );
      final page = (value[wrapper] as List).map(jsonObject).toList();
      rows.addAll(page);
      if (value['has_more'] != true &&
          value['pagination']?['has_more'] != true) {
        return rows;
      }
      if (page.isEmpty) {
        throw const FormatException('Incomplete reference list');
      }
    }
  }

  Future<void> create({bool retry = false}) async {
    if (acting || c.busy || c.loading || c.blocked || (!retry && !c.writable)) {
      return;
    }
    setState(() {
      acting = true;
      action = 'create';
    });
    try {
      JsonObject? body;
      if (!retry) {
        final config = await productionForm(context, 'New export', [
          const PurchaseField(
            'export_type',
            'Dataset',
            kind: 'select',
            required: true,
            initial: 'CUSTOMER_SUMMARY',
            options: exportTypes,
          ),
          const PurchaseField(
            'format',
            'File format',
            kind: 'select',
            required: true,
            initial: 'CSV',
            options: {'CSV': 'CSV spreadsheet', 'JSONL': 'JSON Lines'},
          ),
          PurchaseField(
            'scope_type',
            'Data scope',
            kind: 'select',
            required: true,
            initial: c.app.scope.branchId == null ? 'BUSINESS' : 'BRANCH',
            options: const {
              'BUSINESS': 'Whole business',
              'BRANCH': 'One branch',
            },
          ),
          const PurchaseField(
            'row_limit',
            'Maximum rows',
            kind: 'number',
            required: true,
            initial: 100000,
            min: 1,
            max: 250000,
          ),
        ]);
        if (config == null || !mounted) return;
        body = {...config, 'filter': <String, dynamic>{}};
        if (config['scope_type'] == 'BRANCH') {
          String? branch = c.app.scope.branchId;
          if (branch == null) {
            final sc = SettingsController(c.app, SettingsKind.branches);
            try {
              final branches = await sc.all(
                '/api/v1/vendor/businesses/{businessId}/branches',
                sc.scope,
              );
              if (!mounted) return;
              final selected = await crmChoose(context, 'Export branch', {
                for (final b in branches) '${b['id']}': '${b['name']}',
              });
              if (selected == null || selected.isEmpty || !mounted) return;
              branch = selected.single;
            } finally {
              sc.dispose();
            }
          }
          body['branch_ref'] = branch;
        }
        final filter = body['filter'] as JsonObject;
        if (config['export_type'] == 'SEGMENT_MEMBERS') {
          final rows = await choices('$crmBase/segments', 'segments');
          if (!mounted) return;
          final selected = await crmChoose(context, 'Choose segment', {
            for (final r in rows)
              if (r['status'] != 'ARCHIVED')
                '${r['segment_ref']}': '${r['name']}',
          });
          if (selected == null || selected.isEmpty || !mounted) return;
          filter['segment_ref'] = selected.single;
        }
        if (config['export_type'] == 'CAMPAIGN_PERFORMANCE_SUMMARY') {
          final rows = await choices('$crmBase/campaigns', 'campaigns');
          if (!mounted) return;
          final selected = await crmChoose(context, 'Campaign filter', {
            for (final r in rows) '${r['campaign_ref']}': '${r['name']}',
          }, optional: true);
          if (selected == null || !mounted) return;
          if (selected.isNotEmpty) filter['campaign_ref'] = selected.single;
        }
        if ([
          'INTELLIGENCE_SCORES',
          'CUSTOMER_SUMMARY',
        ].contains(config['export_type'])) {
          final bands = await crmChoose(
            context,
            'Risk bands',
            const {
              'LOW': 'Low',
              'MEDIUM': 'Medium',
              'HIGH': 'High',
              'CRITICAL': 'Critical',
            },
            multiple: true,
            optional: true,
          );
          if (bands == null || !mounted) return;
          if (bands.isNotEmpty) filter['risk_bands'] = bands;
          final states = await crmChoose(
            context,
            'Score availability',
            const {
              'NOT_AVAILABLE': 'Not available',
              'CURRENT': 'Current',
              'STALE': 'Stale',
              'FAILED': 'Failed',
            },
            multiple: true,
            optional: true,
          );
          if (states == null || !mounted) return;
          if (states.isNotEmpty) filter['score_states'] = states;
          if (config['export_type'] == 'INTELLIGENCE_SCORES') {
            final model = await crmChoose(context, 'Score model', const {
              'RFM_V1': 'Recency, frequency and spend',
              'CHURN_V1': 'Churn risk',
            }, optional: true);
            if (model == null || !mounted) return;
            if (model.isNotEmpty) filter['model_key'] = model.single;
          }
        }
        final statuses = switch (config['export_type']) {
          'CUSTOMER_SUMMARY' => const {
            'ACTIVE': 'Active',
            'ARCHIVED': 'Archived',
          },
          'CORPORATE_CUSTOMERS' => const {
            'PROSPECT': 'Prospect',
            'ACTIVE': 'Active',
            'ON_HOLD': 'On hold',
            'INACTIVE': 'Inactive',
            'ARCHIVED': 'Archived',
          },
          'CAMPAIGN_PERFORMANCE_SUMMARY' => const {
            'COMPLETED_WITH_FAILURES': 'Completed with failures',
            'SCHEDULED': 'Scheduled',
            'QUEUED': 'Queued',
            'RUNNING': 'Running',
            'PAUSED': 'Paused',
            'COMPLETED': 'Completed',
            'CANCELLED': 'Cancelled',
            'FAILED': 'Failed',
          },
          _ => const <String, String>{},
        };
        if (statuses.isNotEmpty) {
          final selected = await crmChoose(
            context,
            'Status filter',
            statuses,
            multiple: true,
            optional: true,
          );
          if (selected == null || !mounted) return;
          if (selected.isNotEmpty) filter['statuses'] = selected;
        }
        final confirmation = await showModalBottomSheet<JsonObject>(
          context: context,
          isScrollControlled: true,
          useSafeArea: true,
          showDragHandle: true,
          builder: (_) => OrderFormSheet(
            title: 'Create export?',
            message:
                '${exportTypes[config['export_type']]} - ${config['format']}. Up to ${config['row_limit']} rows from ${config['scope_type'] == 'BUSINESS' ? 'your business' : 'the selected branch'}. The server controls eligible records and exported fields.',
            fields: const [],
            icon: Icons.file_download_outlined,
          ),
        );
        if (confirmation == null || !mounted) return;
      }
      final grant = await verify('crm.export.create', 'Verify export creation');
      if (grant == null || !mounted) return;
      final result = await c.createExport(grant, settings: body);
      if (!mounted) return;
      await message(
        result == null ? 'Export not confirmed' : 'Export queued',
        result == null
            ? c.error ?? 'Try again.'
            : 'Your export is being prepared. Check its status before downloading.',
        success: result != null,
      );
      if (!mounted) return;
      if (result != null) {
        await load();
        if (!mounted) return;
        await Navigator.push(
          context,
          setupRoute<void>(
            CrmExportsScreen(runtime: widget.runtime, id: result['export_ref']),
          ),
        );
        if (mounted) await load();
      } else if (c.pending == null && !c.blocked) {
        await load();
      }
    } catch (_) {
      if (mounted) {
        await message(
          'Export unavailable',
          'The export could not be prepared. Refresh and try again.',
        );
      }
    } finally {
      if (mounted) {
        setState(() {
          acting = false;
          action = null;
        });
      }
    }
  }

  Future<void> cancel({bool retry = false}) async {
    if (acting || c.busy || c.loading) return;
    setState(() {
      acting = true;
      action = 'cancel';
    });
    try {
      if (!retry) {
        final confirm = await showModalBottomSheet<JsonObject>(
          context: context,
          isScrollControlled: true,
          useSafeArea: true,
          showDragHandle: true,
          builder: (_) => OrderFormSheet(
            title: 'Cancel this export?',
            message:
                'Preparation will stop and any saved export file on the server will be revoked.',
            fields: const [],
            icon: Icons.cancel_outlined,
          ),
        );
        if (confirm == null || !mounted) return;
      }
      final result = await c.save(
        method: retry ? null : 'POST',
        route: retry ? null : '$crmBase/exports/{exportId}/cancel',
        params: retry ? const {} : {'exportId': widget.id!},
        body: retry ? null : {'expected_version': c.record!['version']},
      );
      if (!mounted) return;
      await message(
        result == null ? 'Cancellation not confirmed' : 'Export cancelled',
        result == null
            ? c.error ?? 'Try again.'
            : 'This export is no longer available for download.',
        success: result != null,
      );
      if (mounted) await load();
    } finally {
      if (mounted) {
        setState(() {
          acting = false;
          action = null;
        });
      }
    }
  }

  Future<void> download() async {
    if (acting || !c.writable) return;
    setState(() {
      acting = true;
      action = 'download';
    });
    try {
      final grant = await verify(
        'crm.export.download',
        'Verify export download',
      );
      if (grant == null || !mounted) return;
      final bytes = await c.app.client.crmExportFile(widget.id!, grant);
      if (!mounted) return;
      final saved = await saveExportFile(
        bytes,
        'karryam-${c.record!['export_type'].toString().toLowerCase()}-${DateTime.now().millisecondsSinceEpoch}.${c.record!['format'] == 'CSV' ? 'csv' : 'jsonl'}',
      );
      if (saved && mounted) {
        await message(
          'Export saved',
          'Your file has been saved to the location you selected.',
          success: true,
        );
      }
    } catch (_) {
      if (mounted) {
        await message(
          'Download unavailable',
          'The file could not be saved. Refresh the export and verify again.',
        );
      }
    } finally {
      if (mounted) {
        setState(() {
          acting = false;
          action = null;
        });
        await load();
      }
    }
  }

  Widget exportIcon(String state) {
    final color =
        (state == 'READY'
                ? const Color(0xFF16815D)
                : ['FAILED', 'EXPIRED'].contains(state)
                ? const Color(0xFFBA4B51)
                : const Color(0xFF3478BA))
            .forInk(context);
    return Container(
      width: 56,
      height: 56,
      decoration: BoxDecoration(
        color: color.withValues(alpha: .12),
        borderRadius: BorderRadius.circular(18),
      ),
      child: Icon(
        state == 'READY'
            ? Icons.task_outlined
            : state == 'FAILED'
            ? Icons.error_outline_rounded
            : Icons.description_outlined,
        color: color,
        size: 29,
      ),
    );
  }

  @override
  void dispose() {
    c.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => Scaffold(
      appBar: AppBar(
        title: Text(widget.id == null ? 'Exports' : 'Export details'),
        actions: [
          IconButton(
            tooltip: 'Refresh',
            onPressed: acting || c.loading || c.busy ? null : () => load(),
            icon: const Icon(Icons.refresh_rounded),
          ),
        ],
      ),
      body: SafeArea(
        child: Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 680),
            child: ListView(
              padding: const EdgeInsets.all(20),
              children: [
                if (c.loading && c.data == null)
                  const PremiumSkeleton(detail: true)
                else ...[
                  if (c.error != null) SettingsNote(c.error!, warning: true),
                  if (c.blocked)
                    const SettingsNote(
                      'Saved CRM recovery is unavailable. Resolve it before creating an export.',
                      warning: true,
                    ),
                  if (c.pending != null) ...[
                    const SettingsNote(
                      'A saved submission needs confirmation before another action.',
                      warning: true,
                    ),
                    if (c.pendingCreation)
                      SettingsAction(
                        'Verify and retry saved export',
                        busy: acting,
                        onPressed: acting ? null : () => create(retry: true),
                        icon: Icons.refresh_rounded,
                      )
                    else if (RegExp(
                      r'^/api/v1/vendor/crm/exports/[^/]+/cancel$',
                    ).hasMatch(c.pending!['path'] as String))
                      SettingsAction(
                        'Retry saved cancellation',
                        busy: acting,
                        onPressed: acting ? null : () => cancel(retry: true),
                      )
                    else
                      const SettingsNote(
                        'Finish the saved CRM action in its original screen.',
                      ),
                  ],
                  if (widget.id == null) ...[
                    const SettingsHeading(
                      'Your business data',
                      subtitle: 'Prepare a secure CSV or JSON Lines file.',
                    ),
                    SettingsAction(
                      'Create export',
                      icon: Icons.add_rounded,
                      busy: acting && action == 'create',
                      onPressed: acting || !c.writable ? null : () => create(),
                    ),
                    const SizedBox(height: 16),
                    SingleChildScrollView(
                      scrollDirection: Axis.horizontal,
                      child: Row(
                        children: [
                          for (final s in <String?>[null, ...exportStates])
                            Padding(
                              padding: const EdgeInsets.only(right: 8),
                              child: ChoiceChip(
                                label: Text(
                                  s == null
                                      ? 'All'
                                      : s[0] + s.substring(1).toLowerCase(),
                                ),
                                selected: status == s,
                                onSelected: acting || c.loading
                                    ? null
                                    : (_) {
                                        setState(() => status = s);
                                        load();
                                      },
                              ),
                            ),
                        ],
                      ),
                    ),
                    const SizedBox(height: 16),
                    if (c.rows.isEmpty && !c.loading && c.error == null)
                      SettingsCard(
                        child: Column(
                          children: [
                            exportIcon('QUEUED'),
                            const SizedBox(height: 16),
                            const SettingsHeading('No exports yet'),
                            const Text(
                              'Create an export to download your selected CRM records.',
                              textAlign: TextAlign.center,
                            ),
                          ],
                        ),
                      ),
                    for (final row in c.rows)
                      SettingsLink(
                        exportTypes[row['export_type']] ?? 'Export',
                        Icons.description_outlined,
                        subtitle:
                            '${row['format']} - ${row['status'].toString().toLowerCase()} - ${supportDate(row['requested_at'])}',
                        onTap: acting
                            ? null
                            : () async {
                                await Navigator.push(
                                  context,
                                  setupRoute<void>(
                                    CrmExportsScreen(
                                      runtime: widget.runtime,
                                      id: row['export_ref'],
                                    ),
                                  ),
                                );
                                if (mounted) await load();
                              },
                      ),
                    if (c.hasMore)
                      SettingsAction(
                        'Load more',
                        secondary: true,
                        busy: c.loading,
                        onPressed: c.loading || acting
                            ? null
                            : () => load(more: true),
                      ),
                  ] else if (c.record != null) ...[
                    SettingsCard(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          exportIcon(c.record!['status']),
                          const SizedBox(height: 16),
                          SettingsHeading(
                            exportTypes[c.record!['export_type']] ?? 'Export',
                          ),
                          SettingsStatus(c.record!['status']),
                          SettingsFact('Format', c.record!['format']),
                          SettingsFact(
                            'Scope',
                            c.record!['scope'] == 'BUSINESS'
                                ? 'Whole business'
                                : 'Branch',
                          ),
                          SettingsFact(
                            'Rows',
                            '${c.record!['row_count']} / ${c.record!['row_limit']}',
                          ),
                          SettingsFact(
                            'Requested',
                            supportDate(c.record!['requested_at']),
                          ),
                          if (c.record!['completed_at'] != null)
                            SettingsFact(
                              'Completed',
                              supportDate(c.record!['completed_at']),
                            ),
                          if (c.record!['expires_at'] != null)
                            SettingsFact(
                              'Available until',
                              supportDate(c.record!['expires_at']),
                            ),
                          if (c.record!['error_code'] != null)
                            SettingsNote(
                              'Export failed: ${c.record!['error_code']}',
                              warning: true,
                            ),
                        ],
                      ),
                    ),
                    if (['QUEUED', 'RUNNING'].contains(c.record!['status']))
                      const SettingsNote(
                        'Your file is being prepared. Refresh to check progress.',
                      ),
                    if (c.record!['status'] == 'READY')
                      SettingsAction(
                        'Verify and download',
                        icon: Icons.file_download_outlined,
                        busy: acting && action == 'download',
                        onPressed: acting || !c.writable ? null : download,
                      ),
                    if ([
                      'QUEUED',
                      'RUNNING',
                      'READY',
                    ].contains(c.record!['status']))
                      SettingsAction(
                        'Cancel export',
                        secondary: true,
                        danger: true,
                        icon: Icons.cancel_outlined,
                        busy: acting && action == 'cancel',
                        onPressed: acting || !c.writable
                            ? null
                            : () => cancel(),
                      ),
                    if (c.record!['fields'] is List)
                      SettingsCard(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            const SettingsHeading('Included fields'),
                            for (final field in c.record!['fields'] as List)
                              Padding(
                                padding: const EdgeInsets.only(bottom: 8),
                                child: Text('$field'.replaceAll('_', ' ')),
                              ),
                          ],
                        ),
                      ),
                  ],
                ],
              ],
            ),
          ),
        ),
      ),
    ),
  );
}
