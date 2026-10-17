import 'package:flutter/material.dart';
import '../../core/app_mode.dart';
import '../../core/config.dart';
import '../../core/network/provider_session.dart';
import '../../core/theme/theme_extensions.dart';

class MockBadge extends StatelessWidget {
  const MockBadge({super.key});
  Future<void> _choose(BuildContext context) async {
    final current = AppModeConfig.instance.current;
    var selected = current;
    var entry = AppModeConfig.instance.mockEntry;
    final appearance = context
        .dependOnInheritedWidgetOfExactType<ProviderSession>()
        ?.themeController;
    final next = await showModalBottomSheet<({AppMode mode, MockEntry entry})>(
      context: context,
      useSafeArea: true,
      showDragHandle: true,
      isScrollControlled: true,
      builder: (sheet) => StatefulBuilder(
        builder: (sheet, update) => Padding(
          padding: EdgeInsets.fromLTRB(
            24,
            8,
            24,
            24 + MediaQuery.viewInsetsOf(sheet).bottom,
          ),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Flexible(
                child: SingleChildScrollView(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Text(
                        'Data source',
                        style: Theme.of(sheet).textTheme.headlineSmall,
                      ),
                      const SizedBox(height: 12),
                      Text(
                        'Explore with demo data or connect to your account. Changing the source restarts this flow.',
                        style: Theme.of(sheet).textTheme.bodyLarge,
                      ),
                      const SizedBox(height: 24),
                      SegmentedButton<AppMode>(
                        segments: const [
                          ButtonSegment(
                            value: AppMode.mock,
                            icon: Icon(Icons.science_outlined),
                            label: Text('Mock'),
                          ),
                          ButtonSegment(
                            value: AppMode.live,
                            icon: Icon(Icons.cloud_outlined),
                            label: Text('Live'),
                          ),
                        ],
                        selected: {selected},
                        onSelectionChanged: (values) =>
                            update(() => selected = values.single),
                      ),
                      if (selected == AppMode.mock) ...[
                        const SizedBox(height: 20),
                        Text(
                          'Start from',
                          style: Theme.of(sheet).textTheme.titleSmall,
                        ),
                        const SizedBox(height: 10),
                        DropdownButtonFormField<MockEntry>(
                          initialValue: entry,
                          items: const [
                            DropdownMenuItem(
                              value: MockEntry.introduction,
                              child: Text('Introduction'),
                            ),
                            DropdownMenuItem(
                              value: MockEntry.setup,
                              child: Text('Onboarding · prefilled'),
                            ),
                            DropdownMenuItem(
                              value: MockEntry.dashboard,
                              child: Text('Dashboard · skip setup'),
                            ),
                          ],
                          onChanged: (value) {
                            if (value != null) update(() => entry = value);
                          },
                        ),
                      ],
                      const SizedBox(height: 20),
                      Container(
                        padding: const EdgeInsets.all(18),
                        decoration: BoxDecoration(
                          color: sheet.palette.surfaceElevated,
                          borderRadius: BorderRadius.circular(20),
                        ),
                        child: Row(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Icon(
                              selected == AppMode.mock
                                  ? Icons.science_outlined
                                  : Icons.cloud_done_outlined,
                              color: sheet.palette.brandTeal600,
                            ),
                            const SizedBox(width: 12),
                            Expanded(
                              child: Column(
                                crossAxisAlignment: CrossAxisAlignment.start,
                                children: [
                                  Text(
                                    selected == AppMode.mock
                                        ? 'Demo workspace'
                                        : 'Server account',
                                    style: Theme.of(sheet).textTheme.titleSmall,
                                  ),
                                  const SizedBox(height: 6),
                                  Text(
                                    selected == AppMode.mock
                                        ? 'Sample data stays separate from your live account.'
                                        : 'Uses your signed-in account and server data.',
                                    style: Theme.of(sheet).textTheme.bodyMedium,
                                  ),
                                  if (selected == AppMode.live) ...[
                                    const SizedBox(height: 12),
                                    SelectableText(
                                      AppConfig.apiBaseUrl,
                                      style: Theme.of(
                                        sheet,
                                      ).textTheme.bodyMedium,
                                    ),
                                  ],
                                ],
                              ),
                            ),
                          ],
                        ),
                      ),
                      if (appearance != null) ...[
                        const SizedBox(height: 20),
                        Text(
                          'Appearance',
                          style: Theme.of(sheet).textTheme.titleSmall,
                        ),
                        const SizedBox(height: 8),
                        Wrap(
                          spacing: 8,
                          children: [
                            for (final mode in ThemeMode.values)
                              ChoiceChip(
                                label: Text(switch (mode) {
                                  ThemeMode.light => 'Light',
                                  ThemeMode.dark => 'Dark',
                                  ThemeMode.system => 'Device',
                                }),
                                selected: appearance.mode == mode,
                                onSelected: (_) async {
                                  await appearance.setMode(mode);
                                  if (sheet.mounted) update(() {});
                                },
                              ),
                          ],
                        ),
                      ],
                      const SizedBox(height: 16),
                      Text(
                        'Unsaved changes on the current screen will be cleared.',
                        style: Theme.of(sheet).textTheme.bodyMedium,
                      ),
                    ],
                  ),
                ),
              ),
              const SizedBox(height: 20),
              FilledButton(
                onPressed: () =>
                    Navigator.pop(sheet, (mode: selected, entry: entry)),
                child: const Text('Apply'),
              ),
            ],
          ),
        ),
      ),
    );
    if (next != null) {
      await AppModeConfig.instance.setMode(next.mode, mockEntry: next.entry);
    }
  }

  @override
  Widget build(BuildContext context) => AnimatedBuilder(
    animation: AppModeConfig.instance,
    builder: (context, _) => Material(
      type: MaterialType.transparency,
      child: IconButton(
        key: const Key('data-mode-button'),
        tooltip:
            'Data source: ${AppModeConfig.instance.isMock ? 'Mock' : 'Live'}',
        onPressed: () => _choose(context),
        icon: const Icon(Icons.tune_rounded, size: 21),
        style: IconButton.styleFrom(
          foregroundColor: context.palette.text,
          backgroundColor: context.palette.surfaceElevated,
          minimumSize: const Size(48, 48),
        ),
      ),
    ),
  );
}
