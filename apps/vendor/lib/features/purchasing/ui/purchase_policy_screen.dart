import 'package:flutter/material.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../dashboard/ui/dashboard_format.dart';
import '../../settings/ui/settings_design.dart';
import 'purchasing_forms.dart';

class PurchasePolicyScreen extends StatefulWidget {
  const PurchasePolicyScreen({
    super.key,
    required this.policy,
    required this.branches,
  });
  final JsonObject? policy;
  final List<JsonObject> branches;
  @override
  State<PurchasePolicyScreen> createState() => _PurchasePolicyScreenState();
}

class _PurchasePolicyScreenState extends State<PurchasePolicyScreen> {
  late final name = TextEditingController(text: widget.policy?['name'] ?? '');
  late bool selfApproval = widget.policy?['allow_self_approval'] == true;
  late final rules = <JsonObject>[
    for (final r in (widget.policy?['rules'] as List? ?? []))
      {
        for (final key in [
          'branch_id',
          'minimum_amount_minor',
          'maximum_amount_minor',
          'required_approval_count',
          'approval_scope',
          'required_permission_code',
          'step_up_required',
        ])
          if (r[key] != null) key: r[key],
      },
  ];
  @override
  void dispose() {
    name.dispose();
    super.dispose();
  }

  Future<void> edit([int? index]) async {
    final r = index == null ? null : rules[index];
    final result = await purchaseForm(
      context,
      'Approval rule',
      [
        PurchaseField(
          'branch_id',
          'Branch',
          kind: 'select',
          options: purchaseChoices(widget.branches, label: 'display_name'),
          initial: r?['branch_id'],
        ),
        PurchaseField(
          'minimum_amount_minor',
          'Minimum amount',
          kind: 'money',
          required: true,
          initial: r?['minimum_amount_minor'] ?? 0,
        ),
        PurchaseField(
          'maximum_amount_minor',
          'Maximum amount (exclusive)',
          kind: 'money',
          initial: r?['maximum_amount_minor'],
        ),
        PurchaseField(
          'required_approval_count',
          'Required approvals',
          kind: 'number',
          required: true,
          min: 1,
          max: 10,
          initial: r?['required_approval_count'] ?? 1,
        ),
        PurchaseField(
          'approval_scope',
          'Approver scope',
          kind: 'select',
          required: true,
          options: purchaseEnums(['BRANCH_OR_BUSINESS', 'BUSINESS_ONLY']),
          initial: r?['approval_scope'] ?? 'BRANCH_OR_BUSINESS',
        ),
        PurchaseField(
          'required_permission_code',
          'Approval permission',
          kind: 'select',
          required: true,
          options: {
            'vendor.purchasing.approve': 'Purchasing approver',
            'vendor.purchasing.approve.business': 'Business approver',
            'vendor.purchasing.approve.high_value': 'High-value approver',
          },
          initial:
              r?['required_permission_code'] ?? 'vendor.purchasing.approve',
        ),
        PurchaseField(
          'step_up_required',
          'Require OTP verification',
          kind: 'bool',
          initial: r?['step_up_required'],
        ),
      ],
      validate: (body) =>
          body['maximum_amount_minor'] != null &&
              body['maximum_amount_minor'] <= body['minimum_amount_minor']
          ? 'Maximum must exceed minimum.'
          : null,
    );
    if (result != null && mounted) {
      setState(() {
        result.removeWhere((k, v) => v == null);
        if (index == null) {
          rules.add(result);
        } else {
          rules[index] = result;
        }
      });
    }
  }

  void finish() {
    for (var i = 0; i < rules.length; i++) {
      for (var j = i + 1; j < rules.length; j++) {
        final a = rules[i], b = rules[j];
        if (a['branch_id'] == b['branch_id'] &&
            (a['maximum_amount_minor'] == null ||
                b['minimum_amount_minor'] < a['maximum_amount_minor']) &&
            (b['maximum_amount_minor'] == null ||
                a['minimum_amount_minor'] < b['maximum_amount_minor'])) {
          ScaffoldMessenger.of(context).showSnackBar(
            const SnackBar(
              content: Text(
                'Approval ranges for the same branch must not overlap.',
              ),
            ),
          );
          return;
        }
      }
    }
    Navigator.pop(context, {
      'name': name.text.trim(),
      'allow_self_approval': selfApproval,
      'expected_version': widget.policy?['version'] ?? 0,
      'rules': rules,
    });
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Approval policy')),
    bottomNavigationBar: SafeArea(
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: SettingsAction(
          'Review policy',
          onPressed: name.text.trim().isEmpty || rules.isEmpty ? null : finish,
        ),
      ),
    ),
    body: SafeArea(
      child: ListView(
        padding: const EdgeInsets.all(20),
        children: [
          TextField(
            controller: name,
            onChanged: (_) => setState(() {}),
            decoration: const InputDecoration(labelText: 'Policy name'),
          ),
          const SizedBox(height: 18),
          SwitchListTile.adaptive(
            contentPadding: EdgeInsets.zero,
            title: const Text('Allow self-approval'),
            subtitle: const Text(
              'Allow order creators to make approval decisions.',
            ),
            value: selfApproval,
            onChanged: (v) => setState(() => selfApproval = v),
          ),
          const SizedBox(height: 18),
          for (var i = 0; i < rules.length; i++) ...[
            SettingsCard(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    '${rules[i]['branch_id'] == null ? 'All branches' : widget.branches.where((b) => b['id'] == rules[i]['branch_id']).firstOrNull?['display_name'] ?? rules[i]['branch_id']}',
                    style: const TextStyle(
                      fontSize: 18,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  Text(
                    '${rules[i]['required_approval_count']} approvals ? ${dashboardLabel(rules[i]['approval_scope'])}',
                  ),
                  Text(
                    'From ${rules[i]['minimum_amount_minor']} to ${rules[i]['maximum_amount_minor'] ?? 'no upper limit'} (minor units)',
                  ),
                  if (rules[i]['step_up_required'] == true)
                    const Text('OTP verification required'),
                  Row(
                    children: [
                      TextButton.icon(
                        onPressed: () => edit(i),
                        icon: const Icon(Icons.edit_outlined),
                        label: const Text('Edit'),
                      ),
                      TextButton.icon(
                        onPressed: () => setState(() => rules.removeAt(i)),
                        icon: const Icon(Icons.delete_outline_rounded),
                        label: const Text('Remove'),
                      ),
                    ],
                  ),
                ],
              ),
            ),
            const SizedBox(height: 12),
          ],
          SettingsAction(
            'Add approval rule',
            secondary: true,
            icon: Icons.add_rounded,
            onPressed: () => edit(),
          ),
        ],
      ),
    ),
  );
}
