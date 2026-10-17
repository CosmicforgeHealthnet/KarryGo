import 'package:flutter/material.dart';
import '../../offline/ui/offline_updates_screen.dart';
import '../../../core/app_mode.dart';
import '../../../core/config.dart';
import '../../missions/data/provider_operations_api.dart';
import '../../missions/ui/commerce_pickup_screen.dart';
import '../../../core/network/provider_session.dart';
import '../../../core/theme/theme_extensions.dart';
import '../../../shared/widgets/mock_badge.dart';

import '../../../shared/widgets/account_components.dart';
import '../../../shared/widgets/journey_icon.dart';

class AppPreferencesScreen extends StatelessWidget {
  const AppPreferencesScreen({super.key});
  Future<void> _openDemo(BuildContext context) async {
    final session = ProviderSession.of(context);
    final api = ProviderOperationsApi(
      origin: AppConfig.apiBaseUrl,
      client: session.client,
      token: session.accessToken,
      onUncertainEvidence: session.offlineUpdates?.saveEvidence,
    );
    try {
      final initial = await api.read(
        '/api/v1/provider/commerce-missions/by-trip/demo-commerce-trip',
      );
      if (!context.mounted) return;
      await Navigator.of(context).push(
        MaterialPageRoute(
          builder: (_) => CommercePickupScreen(
            api: api,
            tripId: 'demo-commerce-trip',
            initial: initial,
          ),
        ),
      );
    } catch (_) {
      if (context.mounted)
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(
            content: Text('The demo could not be opened. Try again.'),
          ),
        );
    }
  }

  @override
  Widget build(BuildContext context) {
    final session = ProviderSession.of(context);
    final controller = session.themeController;
    final c = context.palette;
    return AccountPage(
      title: 'App preferences',
      symbol: JourneySymbol.preferences,
      description: 'Choose how Karryam looks and connects.',
      children: [
        if (controller != null)
          AnimatedBuilder(
            animation: controller,
            builder: (context, _) => AccountSection(
              title: 'Appearance',
              children: [
                for (final mode in [
                  ThemeMode.light,
                  ThemeMode.dark,
                  ThemeMode.system,
                ])
                  ListTile(
                    contentPadding: const EdgeInsets.symmetric(
                      horizontal: 18,
                      vertical: 8,
                    ),
                    leading: Icon(switch (mode) {
                      ThemeMode.light => Icons.light_mode_outlined,
                      ThemeMode.dark => Icons.dark_mode_outlined,
                      ThemeMode.system => Icons.brightness_auto_outlined,
                    }, color: c.brandTeal600),
                    title: Text(switch (mode) {
                      ThemeMode.light => 'Light',
                      ThemeMode.dark => 'Dark',
                      ThemeMode.system => 'Use device setting',
                    }),
                    trailing: Icon(
                      controller.mode == mode
                          ? Icons.radio_button_checked_rounded
                          : Icons.radio_button_off_rounded,
                      color: controller.mode == mode
                          ? c.brandTeal600
                          : c.textMuted,
                    ),
                    onTap: () => controller.setMode(mode),
                  ),
              ],
            ),
          ),
        const AccountSection(
          title: 'Connection',
          children: [
            AccountMenuTile(
              title: 'Data source',
              subtitle: 'Demo data or your Live account',
              symbol: JourneySymbol.preferences,
              trailing: MockBadge(),
            ),
          ],
        ),
        if (session.offlineUpdates != null)
          AccountSection(
            title: 'Sync',
            children: [
              AccountMenuTile(
                title: 'Saved offline updates',
                subtitle: 'Review pending sync and evidence confirmation',
                symbol: JourneySymbol.review,
                onTap: () => Navigator.of(context).push(
                  MaterialPageRoute(
                    builder: (_) => OfflineUpdatesScreen(
                      controller: session.offlineUpdates!,
                    ),
                  ),
                ),
              ),
            ],
          ),
        if (AppModeConfig.instance.isMock)
          AccountSection(
            title: 'Demo scenarios',
            children: [
              AccountMenuTile(
                title: 'Multi-stop pickup',
                subtitle: 'Try sender verification and parcel collection',
                symbol: JourneySymbol.route,
                onTap: () => _openDemo(context),
              ),
            ],
          ),
      ],
    );
  }
}
