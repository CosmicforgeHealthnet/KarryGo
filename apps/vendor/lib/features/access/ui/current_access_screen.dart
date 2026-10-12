import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/ui/premium_loading.dart';
import '../../settings/state/settings_controller.dart';
import '../../settings/ui/settings_design.dart';

class CurrentAccessScreen extends StatefulWidget {
  const CurrentAccessScreen({super.key, required this.runtime});
  final VendorRuntime runtime;
  @override
  State<CurrentAccessScreen> createState() => _CurrentAccessScreenState();
}

class _CurrentAccessScreenState extends State<CurrentAccessScreen> {
  List<JsonObject> grants = [];
  bool loading = true;
  String? error;
  @override
  void initState() {
    super.initState();
    load();
  }

  Future<void> load() async {
    setState(() {
      loading = true;
      error = null;
      grants = [];
    });
    try {
      final response = await widget.runtime.backend.accessApi.request(
        'GET',
        '/api/v1/vendor/staff/me/access',
      );
      final rows = SettingsController.objects(response.data, 'grants');
      rows.sort(
        (a, b) => '${a['permission_code']}:${a['branch_id']}'.compareTo(
          '${b['permission_code']}:${b['branch_id']}',
        ),
      );
      if (mounted) setState(() => grants = rows);
    } catch (e) {
      if (mounted) setState(() => error = SettingsController.message(e));
    } finally {
      if (mounted) setState(() => loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final colors = Theme.of(context).colorScheme;
    return Scaffold(
      appBar: AppBar(
        title: const Text('My access'),
        actions: [
          IconButton(
            onPressed: loading ? null : load,
            tooltip: 'Refresh access',
            icon: const Icon(Icons.refresh_rounded),
          ),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: load,
        child: ListView(
          physics: const AlwaysScrollableScrollPhysics(),
          padding: const EdgeInsets.fromLTRB(20, 12, 20, 32),
          children: [
            if (loading)
              const PremiumSkeleton(detail: true)
            else if (error != null) ...[
              Icon(
                Icons.cloud_off_rounded,
                size: 48,
                color: colors.onSurfaceVariant,
              ),
              const SizedBox(height: 16),
              Text(error!, textAlign: TextAlign.center),
              const SizedBox(height: 20),
              SettingsAction('Try again', onPressed: load),
            ] else ...[
              SettingsCard(
                child: Row(
                  children: [
                    Icon(
                      Icons.verified_user_outlined,
                      size: 42,
                      color: KarryamColors.tealInk.forInk(context),
                    ),
                    const SizedBox(width: 16),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            'Your permissions',
                            style: Theme.of(context).textTheme.titleLarge,
                          ),
                          const SizedBox(height: 6),
                          Text(
                            'Current role assignments and their branch scope.',
                            style: TextStyle(color: colors.onSurfaceVariant),
                          ),
                        ],
                      ),
                    ),
                  ],
                ),
              ),
              const SizedBox(height: 14),
              Text(
                'Individual actions may also require verification or access to the specific record.',
                style: TextStyle(color: colors.onSurfaceVariant),
              ),
              const SizedBox(height: 24),
              if (grants.isEmpty)
                SettingsCard(
                  child: Column(
                    children: [
                      Icon(
                        Icons.lock_person_outlined,
                        size: 48,
                        color: colors.onSurfaceVariant,
                      ),
                      const SizedBox(height: 14),
                      const Text('No permissions assigned'),
                      const SizedBox(height: 8),
                      const Text(
                        'Ask your business administrator to review your access.',
                        textAlign: TextAlign.center,
                      ),
                    ],
                  ),
                ),
              for (final grant in grants)
                Padding(
                  padding: const EdgeInsets.only(bottom: 12),
                  child: SettingsCard(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Row(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Icon(
                              grant['branch_id'] == null
                                  ? Icons.business_outlined
                                  : Icons.storefront_outlined,
                              color: KarryamColors.tealInk.forInk(context),
                            ),
                            const SizedBox(width: 12),
                            Expanded(
                              child: Text(
                                grant['description']?.toString().isNotEmpty ==
                                        true
                                    ? '${grant['description']}'
                                    : '${grant['permission_code']}',
                                style: const TextStyle(
                                  fontWeight: FontWeight.w600,
                                ),
                              ),
                            ),
                            if (grant['is_high_risk'] == true)
                              Padding(
                                padding: const EdgeInsets.only(left: 8),
                                child: Tooltip(
                                  message: 'High-risk permission',
                                  child: Icon(
                                    Icons.shield_outlined,
                                    color: const Color(
                                      0xFF986013,
                                    ).forInk(context),
                                  ),
                                ),
                              ),
                          ],
                        ),
                        const SizedBox(height: 12),
                        Text(
                          grant['branch_id'] == null
                              ? 'Business-wide'
                              : 'Branch-scoped',
                          style: TextStyle(
                            color: colors.onSurfaceVariant,
                            fontWeight: FontWeight.w600,
                          ),
                        ),
                        if (grant['branch_id'] != null)
                          SelectableText(
                            '${grant['branch_id']}',
                            style: TextStyle(
                              color: colors.onSurfaceVariant,
                              fontSize: 12,
                            ),
                          ),
                        const SizedBox(height: 6),
                        SelectableText(
                          '${grant['permission_code']}',
                          style: TextStyle(
                            color: colors.onSurfaceVariant,
                            fontSize: 12,
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
            ],
          ],
        ),
      ),
    );
  }
}
