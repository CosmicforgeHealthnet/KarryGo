import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/ui/premium_loading.dart';
import '../../onboarding/ui/setup_design.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../support/ui/order_support_screen.dart' show supportDate;
import '../../production/ui/production_form_screen.dart';
import '../../purchasing/ui/purchasing_forms.dart';
import '../../settings/ui/settings_design.dart';
import '../../settings/state/settings_controller.dart';
import '../state/crm_workspace_controller.dart';
import 'crm_design.dart' hide crmBase;
import 'crm_editors.dart';
import 'crm_workspace_screen.dart';

const campaignBase = '$crmBase/campaigns';
const campaignChannels = ['EMAIL', 'PUSH', 'IN_APP', 'SMS'];
const campaignObjectives = [
  'GENERAL_MARKETING',
  'RETENTION',
  'REACTIVATION',
  'LOYALTY',
  'ANNOUNCEMENT',
];
const campaignStates = [
  'DRAFT',
  'PREPARING',
  'PREPARED',
  'PREPARATION_FAILED',
  'ARCHIVED',
];
Color campaignColor(Object? status) => switch (status) {
  'PREPARED' ||
  'CURRENT' ||
  'DELIVERED' ||
  'COMPLETED' => KarryamColors.tealInk,
  'FAILED' || 'FAILED_PERMANENT' || 'PREPARATION_FAILED' => KarryamColors.error,
  'PAUSED' ||
  'BUILDING' ||
  'PREPARING' ||
  'DEFERRED_QUIET_HOURS' ||
  'COMPLETED_WITH_FAILURES' ||
  'FAILED_TRANSIENT' => const Color(0xFFA66E19),
  'QUEUED' ||
  'RUNNING' ||
  'SCHEDULED' ||
  'ACCEPTED' ||
  'HANDED_OFF' => const Color(0xFF326AA2),
  _ => const Color(0xFF738292),
};

class CampaignWorkspaceScreen extends StatefulWidget {
  const CampaignWorkspaceScreen({
    super.key,
    required this.runtime,
    this.campaignId,
    this.section,
    this.snapshotId,
    this.runId,
    this.deliveryId,
  });
  final VendorRuntime runtime;
  final String? campaignId, section, snapshotId, runId, deliveryId;
  @override
  State<CampaignWorkspaceScreen> createState() =>
      _CampaignWorkspaceScreenState();
}

class _CampaignWorkspaceScreenState extends State<CampaignWorkspaceScreen> {
  String get campaignPath => '$campaignBase/{campaignId}';
  String get runPath => '$campaignPath/runs/{runId}';
  String get snapshotPath => '$campaignPath/snapshots/{snapshotId}';
  String get path => widget.campaignId == null
      ? campaignBase
      : widget.section == 'history'
      ? '$runPath/deliveries/{deliveryId}/history'
      : widget.section == 'targets'
      ? '$snapshotPath/targets'
      : widget.section == 'deliveries'
      ? '$runPath/deliveries'
      : widget.deliveryId != null
      ? '$runPath/deliveries/{deliveryId}'
      : widget.runId != null
      ? runPath
      : widget.snapshotId != null
      ? snapshotPath
      : widget.section != null
      ? '$campaignPath/${widget.section}'
      : campaignPath;
  Map<String, String> get params => {
    if (widget.campaignId != null) 'campaignId': widget.campaignId!,
    if (widget.snapshotId != null) 'snapshotId': widget.snapshotId!,
    if (widget.runId != null) 'runId': widget.runId!,
    if (widget.deliveryId != null) 'deliveryId': widget.deliveryId!,
  };
  bool get listing => widget.campaignId == null || widget.section != null;
  bool get campaignDetail =>
      !listing && widget.runId == null && widget.snapshotId == null;
  bool get mock => widget.runtime.mode == VendorDataMode.mock;
  late final c = CrmWorkspaceController(
    widget.runtime,
    path,
    parameters: params,
  );
  JsonObject? get campaign =>
      c.record?['campaign'] is Map ? jsonObject(c.record!['campaign']) : null;
  JsonObject get definition => jsonObject(c.record?['definition'] ?? {});
  bool preparing = false;
  String? action;
  String status = '';
  final segmentNames = <String, String>{};
  String get title => widget.section != null
      ? crmLabel(widget.section!)
      : widget.deliveryId != null
      ? 'Delivery details'
      : widget.runId != null
      ? 'Campaign run'
      : widget.snapshotId != null
      ? 'Audience snapshot'
      : widget.campaignId == null
      ? 'Campaigns'
      : 'Campaign details';
  @override
  void initState() {
    super.initState();
    load();
  }

  @override
  void dispose() {
    c.dispose();
    super.dispose();
  }

  Future<void> load({bool append = false}) async {
    c.readQuery = listing
        ? {
            'limit': '50',
            'offset': append ? '${c.rows.length}' : '0',
            if (widget.campaignId == null && status.isNotEmpty)
              'status': status,
          }
        : {};
    await c.load(append: append);
    if (campaignDetail && c.record != null) {
      for (final ref in [
        definition['source_segment_ref'],
        ...definition['exclusion_segment_refs'] as List? ?? [],
      ]) {
        if (ref == null) continue;
        try {
          final segment = jsonObject(
            await c.fetch(
              '$crmBase/segments/{segmentId}',
              params: {'segmentId': '$ref'},
            ),
          );
          segmentNames['$ref'] = '${segment['name'] ?? 'Unavailable'}';
        } catch (_) {
          segmentNames['$ref'] = 'Unavailable';
        }
      }
      if (mounted) setState(() {});
    }
  }

  Future<void> open({
    String? campaignId,
    String? section,
    String? snapshotId,
    String? runId,
    String? deliveryId,
  }) async {
    await Navigator.push(
      context,
      setupRoute<void>(
        CampaignWorkspaceScreen(
          runtime: widget.runtime,
          campaignId: campaignId ?? widget.campaignId,
          section: section,
          snapshotId: snapshotId,
          runId: runId,
          deliveryId: deliveryId,
        ),
      ),
    );
    if (mounted) await load();
  }

  Future<void> feedback(
    String message, {
    bool success = false,
    bool warning = false,
  }) => showOrderFeedback(
    context,
    title: success ? 'Saved' : 'Review required',
    message: message,
    success: success,
    warning: warning,
  );
  Future<JsonObject?> change(
    String method,
    String route,
    JsonObject body, {
    String message = 'Your campaign is up to date.',
  }) async {
    setState(() => action = route);
    final saved = await c.save(
      method: method,
      route: route,
      body: body,
      params: params,
    );
    if (!mounted) return saved;
    if (saved == null) {
      await feedback(
        c.error ?? 'Change could not be confirmed.',
        warning: c.pending != null,
      );
    } else {
      await feedback(message, success: true);
      if (mounted) await load();
    }
    if (mounted) setState(() => action = null);
    return saved;
  }

  Future<List<JsonObject>> discover(
    String route,
    String key, {
    Map<String, String> query = const {},
  }) async {
    final all = <JsonObject>[];
    for (var offset = 0; ; offset += 100) {
      final data = jsonObject(
        await c.fetch(
          route,
          query: {...query, 'limit': '100', 'offset': '$offset'},
        ),
      );
      final page = c.collection(data[key]);
      all.addAll(page);
      if (data['has_more'] != true &&
          jsonObject(data['pagination'] ?? {})['has_more'] != true) {
        break;
      }
      if (page.isEmpty) {
        throw const FormatException('Incomplete selection list');
      }
    }
    return all;
  }

  Future<void> edit() async {
    if (preparing || !c.writable) return;
    setState(() {
      preparing = true;
      action = 'edit';
    });
    try {
      final source = definition;
      final segments = await discover('$crmBase/segments', 'items');
      final available = {
        for (final r in segments.where((s) => s['status'] == 'ACTIVE'))
          '${r['segment_ref']}': '${r['name']}',
      };
      if (!mounted) return;
      if (available.isEmpty) {
        await feedback(
          'Create and activate a segment before defining a campaign.',
          warning: true,
        );
        return;
      }
      final fields = [
        PurchaseField(
          'name',
          'Campaign name',
          required: true,
          initial: source['name'] ?? (mock ? 'Stay connected' : null),
        ),
        PurchaseField(
          'objective',
          'Objective',
          kind: 'select',
          required: true,
          options: {for (final o in campaignObjectives) o: crmLabel(o)},
          initial: source['objective'] ?? 'GENERAL_MARKETING',
        ),
        if (campaign == null)
          PurchaseField(
            'scope_type',
            'Audience scope',
            kind: 'select',
            required: true,
            options: const {
              'BUSINESS': 'Whole business',
              'BRANCH': 'One branch',
            },
            initial: source['scope_type'] ?? 'BUSINESS',
          ),
        PurchaseField(
          'source_segment_ref',
          'Source segment',
          kind: 'select',
          required: true,
          options: available,
          initial:
              source['source_segment_ref'] ??
              (mock ? available.keys.first : null),
        ),
        PurchaseField(
          'frequency_cap_count',
          'Contact limit',
          kind: 'number',
          min: 1,
          max: 1000,
          initial: jsonObject(source['frequency_policy'] ?? {})['cap'],
        ),
        PurchaseField(
          'frequency_cap_window_minutes',
          'Contact window (minutes)',
          kind: 'number',
          min: 1,
          max: 525600,
          initial: jsonObject(
            source['frequency_policy'] ?? {},
          )['window_minutes'],
        ),
        PurchaseField(
          'quiet_hours_enabled',
          'Respect quiet hours',
          kind: 'bool',
          initial: jsonObject(source['quiet_hours_policy'] ?? {})['enabled'],
        ),
        PurchaseField(
          'quiet_hours_start',
          'Quiet hours start',
          kind: 'clock',
          initial:
              jsonObject(source['quiet_hours_policy'] ?? {})['start'] ??
              (mock ? '22:00' : null),
        ),
        PurchaseField(
          'quiet_hours_end',
          'Quiet hours end',
          kind: 'clock',
          initial:
              jsonObject(source['quiet_hours_policy'] ?? {})['end'] ??
              (mock ? '07:00' : null),
        ),
        PurchaseField(
          'quiet_hours_timezone',
          'Quiet hours timezone',
          initial:
              jsonObject(source['quiet_hours_policy'] ?? {})['timezone'] ??
              (mock ? 'Africa/Lagos' : null),
        ),
        PurchaseField(
          'service_recovery_suppression_enabled',
          'Suppress unresolved service recovery',
          kind: 'bool',
          initial: source['service_recovery_suppression_enabled'] ?? true,
        ),
        PurchaseField(
          'experiment_enabled',
          'Split audience into variants',
          kind: 'bool',
          initial: source['experiment_enabled'],
        ),
        if (campaign != null)
          PurchaseField(
            'change_reason',
            'Reason for change',
            kind: 'notes',
            initial: mock ? 'Updated campaign definition' : null,
          ),
      ];
      final body = await productionForm(
        context,
        campaign == null ? 'New campaign' : 'Edit campaign',
        fields,
        validate: (b) {
          if ('${b['name']}'.trim().length < 2 || '${b['name']}'.length > 120) {
            return 'Use a campaign name between 2 and 120 characters.';
          }
          if ((b['frequency_cap_count'] == null) !=
              (b['frequency_cap_window_minutes'] == null)) {
            return 'Set both the contact limit and window, or leave both empty.';
          }
          if (b['quiet_hours_enabled'] == true &&
              [
                'quiet_hours_start',
                'quiet_hours_end',
                'quiet_hours_timezone',
              ].any((k) => '${b[k] ?? ''}'.trim().isEmpty)) {
            return 'Complete the quiet-hours times and timezone.';
          }
          return null;
        },
      );
      if (body == null || !mounted) return;
      if (campaign != null) {
        body['scope_type'] = source['scope_type'];
        if (source['branch_ref'] != null) {
          body['branch_ref'] = source['branch_ref'];
        }
      }
      if (campaign == null && body['scope_type'] == 'BRANCH') {
        final settings = SettingsController(c.app, SettingsKind.branches);
        final branches = await settings.all(
          '/api/v1/vendor/businesses/{businessId}/branches',
          {'businessId': c.app.scope.vendorId!},
        );
        settings.dispose();
        if (!mounted) return;
        final selected = await crmChoose(
          context,
          'Choose branch',
          {for (final b in branches) '${b['id']}': '${b['name']}'},
          initial: source['branch_ref'] == null
              ? []
              : ['${source['branch_ref']}'],
        );
        if (selected == null || !mounted) return;
        body['branch_ref'] = selected.single;
      }
      final channels = await crmChoose(
        context,
        'Choose channels',
        {for (final channel in campaignChannels) channel: crmLabel(channel)},
        multiple: true,
        initial: List<String>.from(
          source['channels'] as List? ?? (mock ? ['IN_APP'] : []),
        ),
      );
      if (channels == null || !mounted) return;
      body['channels'] = channels;
      final exclusions = await crmChoose(
        context,
        'Exclude segments',
        {
          for (final e in available.entries)
            if (e.key != body['source_segment_ref']) e.key: e.value,
        },
        multiple: true,
        optional: true,
        initial: List<String>.from(
          source['exclusion_segment_refs'] as List? ?? [],
        ),
      );
      if (exclusions == null || !mounted) return;
      if (exclusions.length > 10) {
        await feedback('Choose up to ten exclusion segments.');
        return;
      }
      body['exclusion_segment_refs'] = exclusions;
      if (body['experiment_enabled'] == true) {
        final variants = await Navigator.push<List<JsonObject>>(
          context,
          setupRoute<List<JsonObject>>(
            CampaignVariantsScreen(
              initial: c.collection(source['experiment_variants']),
              mock: mock,
            ),
          ),
        );
        if (variants == null || !mounted) return;
        body['experiment_variants'] = variants;
      } else {
        body['experiment_variants'] = <JsonObject>[];
      }
      if (!mounted) return;
      if (await confirmSettings(
        context,
        'Save campaign?',
        campaign == null
            ? 'This creates a draft. No messages will be sent.'
            : 'This replaces the definition. Prepare a fresh audience before starting a new run.',
      )) {
        if (campaign != null) body['expected_version'] = campaign!['version'];
        final saved = await change(
          campaign == null ? 'POST' : 'PATCH',
          campaign == null ? campaignBase : campaignPath,
          body,
        );
        if (saved != null && widget.campaignId == null && mounted) {
          await open(
            campaignId: '${jsonObject(saved['campaign'])['campaign_ref']}',
          );
        }
      }
    } catch (e) {
      if (mounted) {
        await feedback(
          'Campaign choices could not be loaded. Refresh and try again.',
        );
      }
    } finally {
      if (mounted) {
        setState(() {
          preparing = false;
          action = null;
        });
      }
    }
  }

  Future<void> preview() async {
    if (preparing || !c.writable) return;
    setState(() {
      preparing = true;
      action = 'preview';
    });
    try {
      final result = (await c.app.crmApi.request(
        'POST',
        '$campaignPath/preview',
        pathParameters: params,
        body: VendorPayload({'sample_limit': 25}),
      )).object;
      if (!mounted) return;
      await Navigator.push(
        context,
        setupRoute<void>(
          CampaignEvidenceScreen(title: 'Audience preview', data: result),
        ),
      );
    } catch (e) {
      if (mounted) {
        await feedback(
          'Audience preview is unavailable. Refresh and try again.',
        );
      }
    } finally {
      if (mounted) {
        setState(() {
          preparing = false;
          action = null;
        });
      }
    }
  }

  Future<void> prepare() async {
    if (await confirmSettings(
      context,
      'Prepare this audience?',
      'The server will freeze a new audience snapshot. This does not send messages.',
    )) {
      final saved = await change(
        'POST',
        '$campaignPath/prepare',
        {},
        message:
            'Audience preparation was accepted. Check the snapshot for its current status.',
      );
      if (saved != null && mounted) {
        await open(snapshotId: '${saved['snapshot_ref']}');
      }
    }
  }

  Future<void> createRun() async {
    if (preparing || !c.writable) return;
    setState(() => preparing = true);
    try {
      final actual = jsonObject(await c.fetch(campaignPath, params: params));
      final current = jsonObject(actual['campaign']);
      if (!mounted) return;
      if (current['status'] != 'PREPARED' ||
          current['current_snapshot_ref'] == null) {
        await feedback(
          'Prepare a current audience before creating a run.',
          warning: true,
        );
        return;
      }
      final form = await productionForm(
        context,
        'Campaign message',
        [
          PurchaseField(
            'mode',
            'Delivery timing',
            kind: 'select',
            required: true,
            options: const {'SEND_NOW': 'Send now', 'SCHEDULED': 'Schedule'},
            initial: 'SEND_NOW',
          ),
          PurchaseField(
            'scheduled_for',
            'Scheduled time',
            kind: 'date',
            initial: mock
                ? DateTime.now()
                      .add(const Duration(days: 1))
                      .toUtc()
                      .toIso8601String()
                : null,
          ),
          PurchaseField(
            'title',
            'Message title',
            initial: mock ? 'Something fresh for you' : null,
          ),
          PurchaseField(
            'body',
            'Message',
            kind: 'notes',
            initial: mock ? 'Explore what is new on our menu.' : null,
          ),
          PurchaseField('template_key', 'Template key'),
          PurchaseField('locale', 'Locale', initial: mock ? 'en-NG' : null),
        ],
        validate: (b) {
          if ([
            'template_key',
            'title',
            'body',
          ].every((k) => '${b[k] ?? ''}'.trim().isEmpty)) {
            return 'Enter a title, message or an existing template key.';
          }
          if ('${b['title'] ?? ''}'.length > 200 ||
              '${b['body'] ?? ''}'.length > 4000 ||
              '${b['template_key'] ?? ''}'.length > 120 ||
              '${b['locale'] ?? ''}'.length > 16) {
            return 'The message exceeds the allowed length.';
          }
          if (RegExp(
            r'[<>]',
          ).hasMatch('${b['title'] ?? ''}${b['body'] ?? ''}')) {
            return 'Use plain text for the message.';
          }
          if (b['mode'] == 'SCHEDULED' &&
              (DateTime.tryParse(
                    '${b['scheduled_for']}',
                  )?.isAfter(DateTime.now()) !=
                  true)) {
            return 'Choose a future scheduled time.';
          }
          return null;
        },
      );
      if (form == null || !mounted) return;
      if (form['mode'] == 'SEND_NOW') form.remove('scheduled_for');
      form['snapshot_ref'] = current['current_snapshot_ref'];
      form['expected_version'] = current['version'];
      if (await confirmSettings(
        context,
        form['mode'] == 'SEND_NOW'
            ? 'Start this campaign?'
            : 'Schedule this campaign?',
        'The server rechecks consent, frequency limits, quiet hours and suppression before delivery.',
      )) {
        final saved = await change(
          'POST',
          '$campaignPath/runs',
          form,
          message: form['mode'] == 'SEND_NOW'
              ? 'Your run was accepted. Follow its delivery progress.'
              : 'Your campaign run has been scheduled.',
        );
        if (saved != null && mounted) await open(runId: '${saved['run_ref']}');
      }
    } catch (e) {
      if (mounted) {
        await feedback(
          'The current audience could not be verified. Refresh before sending.',
        );
      }
    } finally {
      if (mounted) {
        setState(() {
          preparing = false;
          action = null;
        });
      }
    }
  }

  Future<void> mutate(String verb) async {
    final row = campaign ?? c.record;
    if (row == null) return;
    if (await confirmSettings(
      context,
      '${crmLabel(verb)} ${widget.runId == null ? 'campaign' : 'run'}?',
      verb == 'archive'
          ? 'This retires the campaign definition. Existing run controls are separate.'
          : verb == 'cancel'
          ? 'Delivery already handed off cannot be recalled.'
          : 'The server will update the remaining delivery work.',
      danger: verb == 'archive' || verb == 'cancel',
    )) {
      await change(
        'POST',
        '${widget.runId == null ? campaignPath : runPath}/$verb',
        {'expected_version': row['version']},
      );
    }
  }

  List<Widget> actions() {
    final row = campaign ?? c.record ?? {};
    Widget button(
      String label,
      VoidCallback onTap, {
      String? route,
      bool danger = false,
      bool secondary = false,
      IconData icon = Icons.arrow_forward_rounded,
    }) => SettingsAction(
      label,
      onPressed: c.writable && !preparing ? onTap : null,
      busy: route != null && action == route,
      secondary: secondary,
      danger: danger,
      icon: icon,
    );
    if (campaignDetail) {
      return [
        if (row['status'] != 'ARCHIVED')
          button(
            'Edit campaign',
            edit,
            route: 'edit',
            secondary: true,
            icon: Icons.edit_outlined,
          ),
        if (row['status'] != 'ARCHIVED')
          button(
            'Preview audience',
            preview,
            route: 'preview',
            secondary: true,
            icon: Icons.people_outline_rounded,
          ),
        if (!['ARCHIVED', 'PREPARING'].contains(row['status']))
          button(
            'Prepare audience',
            prepare,
            route: '$campaignPath/prepare',
            icon: Icons.layers_outlined,
          ),
        if (row['status'] == 'PREPARED' && row['current_snapshot_ref'] != null)
          button(
            'Create campaign run',
            createRun,
            route: '$campaignPath/runs',
            icon: Icons.send_outlined,
          ),
        SettingsLink(
          'Audience snapshots',
          Icons.layers_outlined,
          onTap: () => open(section: 'snapshots'),
        ),
        SettingsLink(
          'Runs & results',
          Icons.insights_outlined,
          onTap: () => open(section: 'runs'),
        ),
        if (row['status'] != 'ARCHIVED')
          button(
            'Archive campaign',
            () => mutate('archive'),
            route: '$campaignPath/archive',
            danger: true,
            icon: Icons.archive_outlined,
          ),
      ];
    }
    if (widget.snapshotId != null && !listing) {
      return [
        SettingsLink(
          'Audience members',
          Icons.people_outline_rounded,
          onTap: () => open(section: 'targets', snapshotId: widget.snapshotId),
        ),
      ];
    }
    if (widget.deliveryId != null && !listing) {
      return [
        SettingsLink(
          'Delivery history',
          Icons.history_rounded,
          onTap: () => open(
            section: 'history',
            runId: widget.runId,
            deliveryId: widget.deliveryId,
          ),
        ),
      ];
    }
    if (widget.runId != null && !listing) {
      return [
        SettingsLink(
          'Deliveries',
          Icons.mark_email_read_outlined,
          onTap: () => open(section: 'deliveries', runId: widget.runId),
        ),
        if (['RUNNING', 'QUEUED', 'SCHEDULED'].contains(row['status']))
          button(
            'Pause run',
            () => mutate('pause'),
            route: '$runPath/pause',
            secondary: true,
            icon: Icons.pause_rounded,
          ),
        if (row['status'] == 'PAUSED')
          button(
            'Resume run',
            () => mutate('resume'),
            route: '$runPath/resume',
            icon: Icons.play_arrow_rounded,
          ),
        if (![
          'COMPLETED',
          'COMPLETED_WITH_FAILURES',
          'CANCELLED',
          'FAILED',
        ].contains(row['status']))
          button(
            'Cancel run',
            () => mutate('cancel'),
            route: '$runPath/cancel',
            danger: true,
            icon: Icons.close_rounded,
          ),
      ];
    }
    return [];
  }

  Widget item(JsonObject row) {
    final icon = widget.section == 'runs'
        ? Icons.send_outlined
        : widget.section == 'snapshots'
        ? Icons.layers_outlined
        : widget.section == 'deliveries'
        ? Icons.mark_email_read_outlined
        : widget.section == 'targets'
        ? Icons.person_outline_rounded
        : Icons.campaign_outlined;
    final label =
        '${row['name'] ?? row['display_label'] ?? (widget.section == 'runs'
                ? '${crmLabel('${row['mode'] ?? 'Campaign'}')} run'
                : widget.section == 'snapshots'
                ? 'Audience snapshot'
                : widget.section == 'deliveries'
                ? crmLabel('${row['channel']}')
                : widget.section == 'history'
                ? 'Delivery update'
                : 'Customer relationship')}';
    return SettingsCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          ListTile(
            contentPadding: EdgeInsets.zero,
            leading: Container(
              width: 48,
              height: 48,
              decoration: BoxDecoration(
                color: campaignColor(
                  row['status'],
                ).forInk(context).withValues(alpha: .10),
                borderRadius: BorderRadius.circular(16),
              ),
              child: Icon(
                icon,
                color: campaignColor(row['status']).forInk(context),
              ),
            ),
            title: Text(
              label,
              style: const TextStyle(fontWeight: FontWeight.w700),
            ),
            subtitle: row['status'] == null
                ? null
                : Text(
                    crmLabel('${row['status']}'),
                    style: TextStyle(
                      color: campaignColor(row['status']).forInk(context),
                    ),
                  ),
            trailing: ['targets', 'history'].contains(widget.section)
                ? null
                : const Icon(Icons.chevron_right_rounded),
            onTap: ['targets', 'history'].contains(widget.section)
                ? null
                : () => widget.campaignId == null
                      ? open(campaignId: '${row['campaign_ref']}')
                      : widget.section == 'runs'
                      ? open(runId: '${row['run_ref']}')
                      : widget.section == 'snapshots'
                      ? open(snapshotId: '${row['snapshot_ref']}')
                      : open(
                          runId: widget.runId,
                          deliveryId: '${row['delivery_ref']}',
                        ),
          ),
          if (widget.section == null && widget.campaignId == null)
            Text(
              '${crmLabel('${row['objective']}')} ? ${crmLabel('${row['scope_type']}')}',
              style: Theme.of(context).textTheme.bodySmall,
            ),
          if (['targets', 'history'].contains(widget.section))
            ...campaignFacts(row),
          for (final key in [
            'target_count',
            'total_deliveries',
            'delivered_count',
            'suppressed_count',
            'failed_count',
          ])
            if (row[key] != null) SettingsFact(crmLabel(key), row[key]),
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => Scaffold(
      appBar: AppBar(title: Text(title)),
      body: SafeArea(
        child: Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 680),
            child: RefreshIndicator(
              onRefresh: load,
              child: ListView(
                physics: const AlwaysScrollableScrollPhysics(),
                padding: const EdgeInsets.fromLTRB(20, 12, 20, 32),
                children: [
                  if (widget.campaignId == null) ...[
                    const Text(
                      'Reach the right customers',
                      style: TextStyle(
                        fontSize: 25,
                        fontWeight: FontWeight.w800,
                      ),
                    ),
                    const SizedBox(height: 6),
                    const Text(
                      'Define an audience, prepare it, then follow each delivery.',
                    ),
                    const SizedBox(height: 20),
                    SettingsAction(
                      'New campaign',
                      icon: Icons.add_rounded,
                      busy: preparing,
                      onPressed: c.writable && !preparing ? edit : null,
                    ),
                    SettingsLink(
                      'Manage segments',
                      Icons.filter_alt_outlined,
                      onTap: () => Navigator.push(
                        context,
                        setupRoute<void>(
                          CrmWorkspaceScreen(
                            runtime: widget.runtime,
                            area: CrmArea.segments,
                          ),
                        ),
                      ),
                    ),
                    SingleChildScrollView(
                      scrollDirection: Axis.horizontal,
                      child: Row(
                        children: [
                          for (final value in ['', ...campaignStates])
                            Padding(
                              padding: const EdgeInsets.only(right: 8),
                              child: ChoiceChip(
                                label: Text(
                                  value.isEmpty ? 'All' : crmLabel(value),
                                ),
                                selected: status == value,
                                onSelected: c.loading
                                    ? null
                                    : (_) {
                                        setState(() => status = value);
                                        load();
                                      },
                              ),
                            ),
                        ],
                      ),
                    ),
                    const SizedBox(height: 18),
                  ],
                  if (c.loading && c.data == null)
                    const PremiumSkeleton(detail: true)
                  else if (c.error != null) ...[
                    SettingsNote(c.error!, warning: true),
                    SettingsAction(
                      'Refresh',
                      onPressed: c.loading ? null : load,
                      secondary: true,
                    ),
                  ],
                  if (c.pending != null) ...[
                    const SettingsNote(
                      'A saved change still needs confirmation. Retry it before making another change.',
                      warning: true,
                    ),
                    SettingsAction(
                      'Retry saved change',
                      busy: c.busy,
                      onPressed: c.busy
                          ? null
                          : () async {
                              final saved = await c.save();
                              if (mounted) {
                                await feedback(
                                  saved == null
                                      ? c.error ?? 'Still not confirmed.'
                                      : 'Your saved change is confirmed.',
                                  success: saved != null,
                                );
                                if (mounted) await load();
                              }
                            },
                    ),
                  ],
                  if (c.data != null && listing && c.rows.isEmpty)
                    SettingsCard(
                      child: Padding(
                        padding: const EdgeInsets.symmetric(vertical: 28),
                        child: Column(
                          children: [
                            Icon(
                              widget.section == 'deliveries'
                                  ? Icons.outgoing_mail
                                  : Icons.campaign_outlined,
                              size: 60,
                              color: const Color(0xFF7660A6).forInk(context),
                            ),
                            const SizedBox(height: 16),
                            Text(
                              widget.campaignId == null
                                  ? 'Your next campaign starts here'
                                  : 'Nothing here yet',
                              style: const TextStyle(
                                fontSize: 20,
                                fontWeight: FontWeight.w700,
                              ),
                            ),
                            const SizedBox(height: 8),
                            Text(
                              widget.campaignId == null
                                  ? 'Create a draft from one of your active customer segments.'
                                  : 'Refresh to see the latest server progress.',
                              textAlign: TextAlign.center,
                            ),
                          ],
                        ),
                      ),
                    ),
                  if (listing)
                    for (final row in c.rows)
                      Padding(
                        padding: const EdgeInsets.only(bottom: 12),
                        child: item(row),
                      ),
                  if (c.hasMore)
                    SettingsAction(
                      'Load more',
                      secondary: true,
                      busy: c.loading,
                      onPressed: c.loading ? null : () => load(append: true),
                    ),
                  if (!listing && c.record != null) ...[
                    SettingsCard(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Icon(
                            widget.runId != null
                                ? Icons.send_outlined
                                : widget.snapshotId != null
                                ? Icons.layers_outlined
                                : Icons.campaign_outlined,
                            size: 38,
                            color: campaignColor(
                              (campaign ?? c.record)?['status'],
                            ).forInk(context),
                          ),
                          const SizedBox(height: 14),
                          Text(
                            '${campaign?['name'] ?? title}',
                            style: const TextStyle(
                              fontSize: 25,
                              fontWeight: FontWeight.w800,
                            ),
                          ),
                          const SizedBox(height: 8),
                          Text(
                            crmLabel(
                              '${(campaign ?? c.record)?['status'] ?? ''}',
                            ),
                            style: TextStyle(
                              color: campaignColor(
                                (campaign ?? c.record)?['status'],
                              ).forInk(context),
                              fontWeight: FontWeight.w700,
                            ),
                          ),
                          const SizedBox(height: 16),
                          ...campaignFacts(campaign ?? c.record!),
                        ],
                      ),
                    ),
                    if (campaignDetail)
                      SettingsCard(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            const SettingsHeading('Audience & safeguards'),
                            SettingsFact(
                              'Source segment',
                              segmentNames['${definition['source_segment_ref']}'] ??
                                  'Loading',
                            ),
                            for (final ref
                                in definition['exclusion_segment_refs']
                                        as List? ??
                                    [])
                              SettingsFact(
                                'Excluded segment',
                                segmentNames['$ref'] ?? 'Loading',
                              ),
                            ...campaignFacts(definition),
                          ],
                        ),
                      ),
                    if (widget.snapshotId != null && !listing)
                      const SettingsNote(
                        'Snapshot eligibility is historical. Consent and safeguards are checked again before sending.',
                      ),
                    if (mock && widget.runId != null)
                      const SettingsNote(
                        'Mock runs simulate consent suppression. They do not send messages.',
                      ),
                    for (final w in actions())
                      Padding(
                        padding: const EdgeInsets.only(top: 12),
                        child: w,
                      ),
                  ],
                ],
              ),
            ),
          ),
        ),
      ),
    ),
  );
}

List<Widget> campaignFacts(JsonObject data, {int depth = 0}) => [
  for (final e in data.entries.where(
    (e) =>
        !e.key.endsWith('_ref') &&
        !e.key.endsWith('_refs') &&
        ![
          'business_id',
          'definition_hash',
          'audience_hash',
          'version',
          'name',
          'status',
          'experiment_bucket',
          'notification_request_ref',
        ].contains(e.key),
  ))
    if (e.value is Map && depth < 2) ...[
      const SizedBox(height: 12),
      SettingsHeading(crmLabel(e.key)),
      ...campaignFacts(jsonObject(e.value), depth: depth + 1),
    ] else if (e.value is List && depth < 2) ...[
      const SizedBox(height: 12),
      SettingsHeading(crmLabel(e.key)),
      for (final v in e.value as List)
        if (v is Map)
          ...campaignFacts(jsonObject(v), depth: depth + 1)
        else
          SettingsFact('', crmLabel('$v')),
    ] else if (e.value is! Map && e.value is! List)
      SettingsFact(
        crmLabel(e.key),
        e.value == null
            ? 'Unavailable'
            : e.key.endsWith('_at') ||
                  e.key == 'scheduled_for' ||
                  e.key == 'evaluation_as_of'
            ? supportDate(e.value)
            : e.value is bool
            ? e.value == true
                  ? 'Yes'
                  : 'No'
            : e.value is String
            ? RegExp(r'^[A-Z][A-Z0-9_]*$').hasMatch('${e.value}')
                  ? crmLabel('${e.value}')
                  : '${e.value}'
            : '${e.value}',
      ),
];

class CampaignEvidenceScreen extends StatelessWidget {
  const CampaignEvidenceScreen({
    super.key,
    required this.title,
    required this.data,
  });
  final String title;
  final JsonObject data;
  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: Text(title)),
    body: SafeArea(
      child: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 680),
          child: ListView(
            padding: const EdgeInsets.all(20),
            children: [
              const SettingsNote(
                'This preview does not send messages or grant permission to contact customers.',
              ),
              SettingsCard(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: campaignFacts(data),
                ),
              ),
            ],
          ),
        ),
      ),
    ),
  );
}

class CampaignVariantsScreen extends StatefulWidget {
  const CampaignVariantsScreen({
    super.key,
    required this.initial,
    required this.mock,
  });
  final List<JsonObject> initial;
  final bool mock;
  @override
  State<CampaignVariantsScreen> createState() => _CampaignVariantsScreenState();
}

class _CampaignVariantsScreenState extends State<CampaignVariantsScreen> {
  late final variants = widget.initial
      .map((v) => Map<String, dynamic>.from(v))
      .toList();
  String? error;
  Future<void> edit([int? index]) async {
    final v = index == null ? <String, dynamic>{} : variants[index];
    final result = await productionForm(
      context,
      'Audience variant',
      [
        PurchaseField(
          'variant_key',
          'Variant key',
          required: true,
          initial:
              v['variant_key'] ??
              (widget.mock ? 'VARIANT_${variants.length + 1}' : null),
        ),
        PurchaseField(
          'display_name',
          'Display name',
          required: true,
          initial:
              v['display_name'] ??
              (widget.mock ? 'Audience ${variants.length + 1}' : null),
        ),
        PurchaseField(
          'allocation_bps',
          'Allocation (basis points)',
          kind: 'number',
          required: true,
          max: 10000,
          initial: v['allocation_bps'] ?? (widget.mock ? 5000 : null),
        ),
        PurchaseField(
          'is_control',
          'Control group',
          kind: 'bool',
          initial: v['is_control'],
        ),
      ],
      validate: (b) =>
          !RegExp(r'^[A-Z0-9_]{1,32}$').hasMatch('${b['variant_key']}')
          ? 'Use an uppercase key of up to 32 letters, digits or underscores.'
          : '${b['display_name']}'.length > 80
          ? 'Keep the display name within 80 characters.'
          : null,
    );
    if (result != null && mounted) {
      setState(() {
        if (index == null) {
          variants.add(result);
        } else {
          variants[index] = result;
        }
        error = null;
      });
    }
  }

  void save() {
    final controls = variants.where((v) => v['is_control'] == true).toList();
    if (variants.isEmpty ||
        variants.length > 6 ||
        controls.length > 1 ||
        variants.where((v) => v['is_control'] != true).isEmpty ||
        variants.where((v) => v['is_control'] != true).length > 5 ||
        controls.any((v) => (v['allocation_bps'] as int) > 5000) ||
        variants.fold<int>(0, (sum, v) => sum + (v['allocation_bps'] as int)) !=
            10000 ||
        variants.map((v) => v['variant_key']).toSet().length !=
            variants.length) {
      setState(
        () => error =
            'Use one to five treatment variants and at most one control (up to 5,000 basis points). Unique keys and allocations totaling 10,000 are required.',
      );
      return;
    }
    Navigator.pop(context, variants);
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Audience split')),
    body: SafeArea(
      child: ListView(
        padding: const EdgeInsets.all(20),
        children: [
          const SettingsNote(
            '10,000 basis points = 100%. Control customers are held out from delivery.',
          ),
          for (var i = 0; i < variants.length; i++)
            SettingsCard(
              child: Column(
                children: [
                  ListTile(
                    contentPadding: EdgeInsets.zero,
                    title: Text('${variants[i]['display_name']}'),
                    subtitle: Text(
                      '${variants[i]['allocation_bps']} basis points${variants[i]['is_control'] == true ? ' ? Control' : ''}',
                    ),
                    trailing: IconButton(
                      icon: const Icon(Icons.edit_outlined),
                      onPressed: () => edit(i),
                    ),
                  ),
                  SettingsAction(
                    'Remove variant',
                    secondary: true,
                    danger: true,
                    onPressed: () => setState(() => variants.removeAt(i)),
                  ),
                ],
              ),
            ),
          if (error != null) SettingsNote(error!, warning: true),
          if (variants.length < 6)
            SettingsAction(
              'Add variant',
              secondary: true,
              icon: Icons.add_rounded,
              onPressed: edit,
            ),
          const SizedBox(height: 12),
          SettingsAction(
            'Use audience split',
            onPressed: variants.isEmpty ? null : save,
          ),
        ],
      ),
    ),
  );
}
