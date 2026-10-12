import 'dart:async';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/ui/premium_loading.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../orders/ui/order_feedback_dialog.dart';
import '../../settings/ui/settings_design.dart';
import '../state/taxonomy_workspace_controller.dart';

class TaxonomyWorkspaceScreen extends StatefulWidget {
  const TaxonomyWorkspaceScreen({
    super.key,
    required this.runtime,
    required this.sourceType,
    required this.sourceRef,
    required this.name,
  });
  final VendorRuntime runtime;
  final String sourceType, sourceRef, name;
  @override
  State<TaxonomyWorkspaceScreen> createState() =>
      _TaxonomyWorkspaceScreenState();
}

class _TaxonomyWorkspaceScreenState extends State<TaxonomyWorkspaceScreen> {
  late final c = TaxonomyWorkspaceController(
    widget.runtime,
    widget.sourceType,
    widget.sourceRef,
  );
  @override
  void initState() {
    super.initState();
    unawaited(c.load());
  }

  @override
  void dispose() {
    c.dispose();
    super.dispose();
  }

  Future<void> save({
    JsonObject? facet,
    List<JsonObject>? candidates,
    String? reviewed,
  }) async {
    final ok = await c.save(
      facetCode: facet?['Code'],
      candidates: candidates,
      reviewed: reviewed,
    );
    if (!mounted) return;
    await showOrderFeedback(
      context,
      title: ok ? 'Classification saved' : 'Check your classification',
      message: c.error ?? 'Your categories have been updated.',
      success: ok,
      warning: !ok,
    );
  }

  Future<void> edit(JsonObject facet) async {
    final reviewed = c.reviewToken();
    final candidates = await Navigator.of(context).push<List<JsonObject>>(
      MaterialPageRoute(
        builder: (_) => _MappingEditor(controller: c, facet: facet),
      ),
    );
    if (!mounted || candidates == null) return;
    final confirmed = await confirmSettings(
      context,
      'Save categories',
      candidates.isEmpty
          ? 'Remove every active mapping in ${facet['DisplayName']} for this item?'
          : 'Replace ${facet['DisplayName']} with your selected categories? Other facets stay unchanged.',
    );
    if (confirmed && mounted) {
      await save(facet: facet, candidates: candidates, reviewed: reviewed);
    }
  }

  Future<void> detail(JsonObject node) async {
    await settingsSheet<void>(context, _NodeDetail(controller: c, node: node));
  }

  Widget empty(String text, IconData icon) => Row(
    children: [
      Icon(
        icon,
        size: 28,
        color: Theme.of(context).colorScheme.onSurfaceVariant,
      ),
      const SizedBox(width: 14),
      Expanded(child: Text(text)),
    ],
  );
  Widget assigned(
    List<JsonObject> rows, {
    required bool dietary,
  }) => SettingsCard(
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            Icon(
              dietary ? Icons.verified_outlined : Icons.sell_outlined,
              color:
                  (dietary ? const Color(0xFF7660A6) : const Color(0xFFA66028))
                      .forInk(context),
            ),
            const SizedBox(width: 12),
            Text(
              dietary ? 'Dietary claims' : 'Assigned tags',
              style: const TextStyle(fontSize: 18, fontWeight: FontWeight.w700),
            ),
          ],
        ),
        const SizedBox(height: 16),
        if (rows.isEmpty)
          empty(
            dietary ? 'No dietary claims assigned.' : 'No tags assigned.',
            dietary ? Icons.fact_check_outlined : Icons.label_off_outlined,
          ),
        for (final r in rows)
          Padding(
            padding: const EdgeInsets.only(bottom: 14),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  '${r['display_name']}',
                  style: const TextStyle(fontWeight: FontWeight.w700),
                ),
                const SizedBox(height: 6),
                SettingsStatus(r['status']),
                if (dietary) ...[
                  const SizedBox(height: 6),
                  Text(dashboardLabel(r['verification_policy'])),
                  if (r['risk_level'] != null)
                    Text('Risk: ${dashboardLabel(r['risk_level'])}'),
                  if (r['expires_at'] != null)
                    Text('Expires: ${r['expires_at']}'),
                ],
              ],
            ),
          ),
      ],
    ),
  );
  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: c,
    builder: (context, _) => Scaffold(
      appBar: AppBar(
        title: const Text('Classification'),
        actions: [
          IconButton(
            tooltip: 'Refresh classification',
            onPressed: c.loading || c.busy ? null : c.load,
            icon: const Icon(Icons.refresh_rounded),
          ),
        ],
      ),
      body: SafeArea(
        child: ListView(
          padding: const EdgeInsets.fromLTRB(20, 12, 20, 32),
          children: [
            if (c.loading)
              const PremiumSkeleton(detail: true)
            else ...[
              SettingsCard(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        Container(
                          padding: const EdgeInsets.all(14),
                          decoration: BoxDecoration(
                            color: Theme.of(
                              context,
                            ).colorScheme.surfaceContainerHighest,
                            borderRadius: BorderRadius.circular(20),
                          ),
                          child: Icon(
                            c.namespace == 'FOOD'
                                ? Icons.restaurant_rounded
                                : Icons.category_rounded,
                            size: 30,
                            color: KarryamColors.tealInk.forInk(context),
                          ),
                        ),
                        const SizedBox(width: 14),
                        Expanded(
                          child: Text(
                            widget.name,
                            style: const TextStyle(
                              fontSize: 24,
                              fontWeight: FontWeight.w700,
                            ),
                          ),
                        ),
                      ],
                    ),
                    if (c.version != null) ...[
                      const SizedBox(height: 16),
                      Text(
                        '${c.namespace == 'FOOD' ? 'Food' : 'Shop'} classification ? Published version ${c.version!['VersionNumber']}',
                      ),
                    ],
                  ],
                ),
              ),
              if (c.error != null) ...[
                const SizedBox(height: 16),
                SettingsCard(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Icon(
                        Icons.info_outline_rounded,
                        color: const Color(0xFF986013).forInk(context),
                      ),
                      const SizedBox(height: 10),
                      Text(c.error!),
                      const SizedBox(height: 12),
                      SettingsAction(
                        'Refresh classification',
                        secondary: true,
                        onPressed: c.busy ? null : c.load,
                      ),
                    ],
                  ),
                ),
              ],
              if (c.pending != null) ...[
                const SizedBox(height: 16),
                SettingsCard(
                  child: Column(
                    children: [
                      const Text(
                        'A saved change needs confirmation. Retry sends the same categories and request key.',
                      ),
                      const SizedBox(height: 16),
                      SettingsAction(
                        'Retry saved change',
                        busy: c.busy,
                        onPressed: c.blocked || c.denied ? null : () => save(),
                      ),
                    ],
                  ),
                ),
              ],
              if (c.version != null && !c.stale) ...[
                const SizedBox(height: 24),
                const Text(
                  'Categories',
                  style: TextStyle(fontSize: 22, fontWeight: FontWeight.w700),
                ),
                const SizedBox(height: 14),
                if (c.unresolvedMappings)
                  SettingsCard(
                    child: empty(
                      'Some saved categories belong to an earlier publication. Editing is unavailable until these mappings are reviewed.',
                      Icons.history_rounded,
                    ),
                  ),
                if (c.facets.where((f) => f['Status'] == 'ACTIVE').isEmpty)
                  SettingsCard(
                    child: empty(
                      'No published categories available.',
                      Icons.account_tree_outlined,
                    ),
                  ),
                for (final f in c.facets.where(
                  (f) => f['Status'] == 'ACTIVE',
                )) ...[
                  SettingsCard(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Row(
                          children: [
                            Expanded(
                              child: Text(
                                '${f['DisplayName']}',
                                style: const TextStyle(
                                  fontSize: 19,
                                  fontWeight: FontWeight.w700,
                                ),
                              ),
                            ),
                            IconButton(
                              tooltip: 'Edit ${f['DisplayName']}',
                              onPressed: c.writable && !c.unresolvedMappings
                                  ? () => edit(f)
                                  : null,
                              icon: const Icon(Icons.edit_outlined),
                            ),
                          ],
                        ),
                        if ((f['Description'] ?? '') != '')
                          Text('${f['Description']}'),
                        const SizedBox(height: 12),
                        if (c.activeMappings(f['ID']).isEmpty)
                          empty(
                            'No categories selected.',
                            Icons.folder_open_rounded,
                          ),
                        for (final m in c.activeMappings(f['ID']))
                          Builder(
                            builder: (context) {
                              final node = c.nodes.firstWhere(
                                (n) => n['ID'] == m['NodeID'],
                              );
                              return ListTile(
                                contentPadding: EdgeInsets.zero,
                                leading: Icon(
                                  m['MappingType'] == 'PRIMARY'
                                      ? Icons.star_outline_rounded
                                      : Icons.account_tree_outlined,
                                  color:
                                      (m['MappingType'] == 'PRIMARY'
                                              ? const Color(0xFFA66028)
                                              : KarryamColors.tealInk)
                                          .forInk(context),
                                ),
                                title: Text('${node['DisplayName']}'),
                                subtitle: Text(
                                  dashboardLabel(m['MappingType']),
                                ),
                                trailing: const Icon(
                                  Icons.chevron_right_rounded,
                                ),
                                onTap: () => detail(node),
                              );
                            },
                          ),
                        TextButton.icon(
                          onPressed: () => Navigator.of(context).push<void>(
                            MaterialPageRoute(
                              builder: (_) => _MappingEditor(
                                controller: c,
                                facet: f,
                                browse: true,
                              ),
                            ),
                          ),
                          icon: const Icon(Icons.search_rounded),
                          label: const Text('Browse categories'),
                        ),
                      ],
                    ),
                  ),
                  const SizedBox(height: 14),
                ],
                assigned(c.tags, dietary: false),
                const SizedBox(height: 14),
                assigned(c.assignments, dietary: true),
                const SizedBox(height: 12),
                const Text(
                  'Tags and dietary claims are managed by the platform.',
                  style: TextStyle(fontSize: 12),
                ),
                if (c.claims.isNotEmpty)
                  TextButton.icon(
                    onPressed: () => settingsSheet<void>(
                      context,
                      Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          const SettingsHeading('Dietary claim definitions'),
                          for (final r in c.claims.where(
                            (r) => r['Status'] == 'ACTIVE',
                          ))
                            ListTile(
                              contentPadding: EdgeInsets.zero,
                              title: Text('${r['DisplayName']}'),
                              subtitle: Text(
                                '${dashboardLabel(r['VerificationPolicy'])} ? ${dashboardLabel(r['RiskLevel'])}',
                              ),
                            ),
                        ],
                      ),
                    ),
                    icon: const Icon(Icons.fact_check_outlined),
                    label: const Text('View dietary claim definitions'),
                  ),
              ],
            ],
          ],
        ),
      ),
    ),
  );
}

class _MappingEditor extends StatefulWidget {
  const _MappingEditor({
    required this.controller,
    required this.facet,
    this.browse = false,
  });
  final TaxonomyWorkspaceController controller;
  final JsonObject facet;
  final bool browse;
  @override
  State<_MappingEditor> createState() => _MappingEditorState();
}

class _MappingEditorState extends State<_MappingEditor> {
  final search = TextEditingController();
  final selected = <String, JsonObject>{};
  String query = '';
  @override
  void initState() {
    super.initState();
    for (final m in widget.controller.activeMappings(widget.facet['ID'])) {
      final node = widget.controller.nodes.firstWhere(
        (n) => n['ID'] == m['NodeID'],
      );
      selected[node['CanonicalCode']] = {
        'node_code': node['CanonicalCode'],
        'mapping_type': m['MappingType'],
        'confidence_basis': m['ConfidenceBasis'],
        if (m['ConfidenceScore'] != null)
          'confidence_score': m['ConfidenceScore'],
        if (m['Metadata'] != null) 'metadata': m['Metadata'],
      };
    }
  }

  @override
  void dispose() {
    search.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final nodes = widget.controller.nodes
        .where(
          (n) =>
              n['FacetID'] == widget.facet['ID'] &&
              n['Status'] == 'ACTIVE' &&
              ('${n['DisplayName']} ${n['CanonicalCode']} ${n['Description']}')
                  .toLowerCase()
                  .contains(query.toLowerCase()),
        )
        .toList();
    return Scaffold(
      appBar: AppBar(title: Text('${widget.facet['DisplayName']}')),
      body: SafeArea(
        child: Column(
          children: [
            Padding(
              padding: const EdgeInsets.all(20),
              child: Column(
                children: [
                  TextField(
                    controller: search,
                    onChanged: (s) => setState(() => query = s),
                    decoration: const InputDecoration(
                      hintText: 'Search categories',
                      prefixIcon: Icon(Icons.search_rounded),
                    ),
                  ),
                  const SizedBox(height: 12),
                  Text(
                    widget.browse
                        ? 'Explore the published categories.'
                        : '${selected.length} selected ? One primary category per facet',
                  ),
                ],
              ),
            ),
            Expanded(
              child: ListView.builder(
                padding: const EdgeInsets.symmetric(horizontal: 20),
                itemCount: nodes.isEmpty ? 1 : nodes.length,
                itemBuilder: (context, i) {
                  if (nodes.isEmpty) {
                    return const SettingsCard(
                      child: Text(
                        'No matching categories. Try another search.',
                      ),
                    );
                  }
                  final node = nodes[i];
                  final code = node['CanonicalCode'] as String;
                  final chosen = selected[code];
                  final parent = widget.controller.nodes
                      .where((n) => n['ID'] == node['ParentID'])
                      .firstOrNull;
                  return Padding(
                    padding: const EdgeInsets.only(bottom: 12),
                    child: SettingsCard(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Row(
                            children: [
                              if (!widget.browse)
                                Checkbox(
                                  value: chosen != null,
                                  onChanged: (value) => setState(() {
                                    if (value == true) {
                                      selected[code] = {
                                        'node_code': code,
                                        'mapping_type':
                                            selected.values.any(
                                              (m) =>
                                                  m['mapping_type'] ==
                                                  'PRIMARY',
                                            )
                                            ? 'SECONDARY'
                                            : 'PRIMARY',
                                        'confidence_basis': 'MANUAL',
                                      };
                                    } else {
                                      selected.remove(code);
                                    }
                                  }),
                                ),
                              Expanded(
                                child: Text(
                                  '${node['DisplayName']}',
                                  style: const TextStyle(
                                    fontSize: 17,
                                    fontWeight: FontWeight.w700,
                                  ),
                                ),
                              ),
                              IconButton(
                                tooltip: 'Category details',
                                onPressed: () => settingsSheet<void>(
                                  context,
                                  _NodeDetail(
                                    controller: widget.controller,
                                    node: node,
                                  ),
                                ),
                                icon: const Icon(Icons.info_outline_rounded),
                              ),
                            ],
                          ),
                          if (parent != null)
                            Text('In ${parent['DisplayName']}'),
                          if ((node['Description'] ?? '') != '')
                            Text('${node['Description']}'),
                          if (chosen != null && !widget.browse)
                            Wrap(
                              spacing: 8,
                              children: [
                                for (final role in [
                                  'PRIMARY',
                                  'SECONDARY',
                                  'DISCOVERY',
                                ])
                                  ChoiceChip(
                                    label: Text(dashboardLabel(role)),
                                    selected: chosen['mapping_type'] == role,
                                    onSelected: (_) => setState(() {
                                      if (role == 'PRIMARY') {
                                        for (final entry in selected.values) {
                                          if (entry['mapping_type'] ==
                                              'PRIMARY') {
                                            entry['mapping_type'] = 'SECONDARY';
                                            entry['confidence_basis'] =
                                                'MANUAL';
                                            entry.remove('confidence_score');
                                          }
                                        }
                                      }
                                      chosen['mapping_type'] = role;
                                      chosen['confidence_basis'] = 'MANUAL';
                                      chosen.remove('confidence_score');
                                    }),
                                  ),
                              ],
                            ),
                        ],
                      ),
                    ),
                  );
                },
              ),
            ),
            if (!widget.browse)
              Padding(
                padding: const EdgeInsets.all(20),
                child: SettingsAction(
                  'Review categories',
                  onPressed: () =>
                      Navigator.pop(context, selected.values.toList()),
                ),
              ),
          ],
        ),
      ),
    );
  }
}

class _NodeDetail extends StatefulWidget {
  const _NodeDetail({required this.controller, required this.node});
  final TaxonomyWorkspaceController controller;
  final JsonObject node;
  @override
  State<_NodeDetail> createState() => _NodeDetailState();
}

class _NodeDetailState extends State<_NodeDetail> {
  late final future = widget.controller.nodeDetail(widget.node['ID']);
  @override
  Widget build(BuildContext context) => FutureBuilder<JsonObject>(
    future: future,
    builder: (context, snapshot) {
      if (snapshot.connectionState != ConnectionState.done) {
        return const PremiumSkeleton(rows: 2);
      }
      if (snapshot.hasError) {
        return const Text(
          'Category details could not be loaded. Close and try again.',
        );
      }
      final node = jsonObject(snapshot.data!['node']);
      final terms = widget.controller.list(snapshot.data!['terms']);
      if (node['ID'] != widget.node['ID'] ||
          node['VersionID'] != widget.controller.version?['ID']) {
        return const Text('This category changed. Refresh classification.');
      }
      return Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SettingsHeading('${node['DisplayName']}'),
          Text('${node['Description'] ?? ''}'),
          const SizedBox(height: 14),
          SettingsStatus(node['Status']),
          if (terms.isNotEmpty) ...[
            const SizedBox(height: 20),
            const Text(
              'Also known as',
              style: TextStyle(fontWeight: FontWeight.w700),
            ),
            for (final term in terms)
              ListTile(
                contentPadding: EdgeInsets.zero,
                title: Text('${term['Term']}'),
                subtitle: Text(
                  '${dashboardLabel(term['RelationshipType'])} ? ${term['Locale']}',
                ),
              ),
          ],
        ],
      );
    },
  );
}
