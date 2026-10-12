import 'package:flutter/material.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'vendor_theme.dart';

class VendorAppearance extends ChangeNotifier {
  VendorAppearance({FlutterSecureStorage? storage})
    : storage = storage ?? const FlutterSecureStorage();
  final FlutterSecureStorage storage;
  ThemeMode mode = ThemeMode.light;
  String? error;
  bool saving = false, disposed = false;
  void emit() {
    if (!disposed) notifyListeners();
  }

  @override
  void dispose() {
    disposed = true;
    super.dispose();
  }

  Future<void> load() async {
    try {
      final saved = await storage.read(key: 'vendor.appearance');
      mode =
          ThemeMode.values.where((m) => m.name == saved).firstOrNull ??
          ThemeMode.light;
    } catch (_) {
      mode = ThemeMode.light;
    }
    emit();
  }

  Future<void> select(ThemeMode choice) async {
    if (saving || choice == mode) return;
    final previous = mode;
    mode = choice;
    saving = true;
    error = null;
    emit();
    try {
      await storage.write(key: 'vendor.appearance', value: choice.name);
    } catch (_) {
      mode = previous;
      error = 'Appearance could not be saved. Try again.';
    } finally {
      saving = false;
      emit();
    }
  }
}

class AppearanceScope extends InheritedNotifier<VendorAppearance> {
  const AppearanceScope({
    super.key,
    required VendorAppearance controller,
    required super.child,
  }) : super(notifier: controller);
  static VendorAppearance? maybeOf(BuildContext context) =>
      context.dependOnInheritedWidgetOfExactType<AppearanceScope>()?.notifier;
}

Future<void> showAppearance(BuildContext context) => showModalBottomSheet<void>(
  context: context,
  useSafeArea: true,
  isScrollControlled: true,
  backgroundColor: Colors.transparent,
  builder: (context) {
    final controller = AppearanceScope.maybeOf(context);
    if (controller == null) return const SizedBox.shrink();
    return AnimatedBuilder(
      animation: controller,
      builder: (context, _) {
        final brightness = switch (controller.mode) {
          ThemeMode.system => MediaQuery.platformBrightnessOf(context),
          ThemeMode.light => Brightness.light,
          ThemeMode.dark => Brightness.dark,
        };
        return AnimatedTheme(
          duration: MediaQuery.disableAnimationsOf(context)
              ? Duration.zero
              : const Duration(milliseconds: 260),
          curve: Curves.easeInOutCubic,
          data: vendorTheme(brightness: brightness),
          child: Builder(
            builder: (context) => Material(
              color: Theme.of(context).colorScheme.surface,
              borderRadius: const BorderRadius.vertical(
                top: Radius.circular(28),
              ),
              child: ConstrainedBox(
                constraints: BoxConstraints(
                  maxHeight: MediaQuery.sizeOf(context).height * .85,
                ),
                child: SingleChildScrollView(
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Container(
                        width: 36,
                        height: 4,
                        margin: const EdgeInsets.symmetric(vertical: 16),
                        decoration: BoxDecoration(
                          color: Theme.of(context).colorScheme.outline,
                          borderRadius: BorderRadius.circular(4),
                        ),
                      ),
                      const AppearancePicker(),
                    ],
                  ),
                ),
              ),
            ),
          ),
        );
      },
    );
  },
);

class AppearancePicker extends StatelessWidget {
  const AppearancePicker({super.key});
  @override
  Widget build(BuildContext context) {
    final controller = AppearanceScope.maybeOf(context);
    if (controller == null) return const SizedBox.shrink();
    return Padding(
      padding: const EdgeInsets.fromLTRB(24, 0, 24, 28),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            'Appearance',
            style: Theme.of(
              context,
            ).textTheme.headlineSmall?.copyWith(fontWeight: FontWeight.w700),
          ),
          const SizedBox(height: 8),
          const Text('Choose your preferred look.'),
          const SizedBox(height: 22),
          for (final option in [
            (
              ThemeMode.system,
              'System',
              Icons.brightness_auto_outlined,
              'Follow your phone’s appearance.',
            ),
            (
              ThemeMode.light,
              'Light',
              Icons.light_mode_outlined,
              'A bright, clean workspace.',
            ),
            (
              ThemeMode.dark,
              'Dark',
              Icons.dark_mode_outlined,
              'A softer view in low light.',
            ),
          ])
            Padding(
              padding: const EdgeInsets.only(bottom: 10),
              child: Material(
                color: controller.mode == option.$1
                    ? KarryamColors.mint.forSurface(context)
                    : Theme.of(context).colorScheme.surface,
                shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(20),
                  side: BorderSide(
                    color: controller.mode == option.$1
                        ? KarryamColors.teal
                        : Theme.of(context).colorScheme.outlineVariant,
                  ),
                ),
                child: InkWell(
                  borderRadius: BorderRadius.circular(20),
                  onTap: controller.saving
                      ? null
                      : () => controller.select(option.$1),
                  child: Padding(
                    padding: const EdgeInsets.all(18),
                    child: Row(
                      children: [
                        Icon(
                          option.$3,
                          color: KarryamColors.tealInk.forInk(context),
                        ),
                        const SizedBox(width: 14),
                        Expanded(
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              Text(
                                option.$2,
                                style: const TextStyle(
                                  fontWeight: FontWeight.w700,
                                ),
                              ),
                              const SizedBox(height: 4),
                              Text(
                                option.$4,
                                style: Theme.of(context).textTheme.bodySmall,
                              ),
                            ],
                          ),
                        ),
                        if (controller.mode == option.$1)
                          Icon(
                            Icons.check_circle_rounded,
                            color: KarryamColors.tealInk.forInk(context),
                          ),
                      ],
                    ),
                  ),
                ),
              ),
            ),
          if (controller.error != null)
            Padding(
              padding: const EdgeInsets.only(top: 8),
              child: Text(
                controller.error!,
                style: TextStyle(color: Theme.of(context).colorScheme.error),
              ),
            ),
        ],
      ),
    );
  }
}
