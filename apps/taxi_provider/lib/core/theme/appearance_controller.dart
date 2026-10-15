import 'package:flutter/material.dart';
import 'package:shared_preferences/shared_preferences.dart';

class AppearanceController extends ChangeNotifier {
  static const preferenceKey = 'taxi_appearance';
  ThemeMode _mode = ThemeMode.system;
  ThemeMode get mode => _mode;
  bool _changed = false;
  bool _disposed = false;
  Future<void> _writes = Future.value();

  Future<void> load() async {
    try {
      final preferences = await SharedPreferences.getInstance();
      if (_disposed || _changed) return;
      final saved = preferences.getString(preferenceKey);
      _mode = ThemeMode.values.firstWhere(
        (mode) => mode.name == saved,
        orElse: () => ThemeMode.system,
      );
      notifyListeners();
    } catch (_) {
      /* Retain the system default if storage is unavailable. */
    }
  }

  Future<bool> select(ThemeMode mode) {
    _changed = true;
    _mode = mode;
    notifyListeners();
    // Serialize rapid changes so the persisted preference matches the last tap.
    final result = _writes.then((_) async {
      try {
        final preferences = await SharedPreferences.getInstance();
        return await preferences.setString(preferenceKey, mode.name);
      } catch (_) {
        return false;
      }
    });
    _writes = result.then((_) {});
    return result;
  }

  @override
  void dispose() {
    _disposed = true;
    super.dispose();
  }
}

class AppearanceScope extends InheritedNotifier<AppearanceController> {
  const AppearanceScope({
    super.key,
    required AppearanceController controller,
    required super.child,
  }) : super(notifier: controller);
  static AppearanceController? maybeOf(BuildContext context) =>
      context.dependOnInheritedWidgetOfExactType<AppearanceScope>()?.notifier;
}

class AppearanceSetting extends StatelessWidget {
  const AppearanceSetting({super.key});
  @override
  Widget build(BuildContext context) {
    final controller = AppearanceScope.maybeOf(context);
    if (controller == null) return const SizedBox.shrink();
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(18),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Row(
              children: [
                Icon(Icons.brightness_6_outlined),
                SizedBox(width: 12),
                Text(
                  'Appearance',
                  style: TextStyle(fontSize: 17, fontWeight: FontWeight.w600),
                ),
              ],
            ),
            const SizedBox(height: 8),
            const Text('Choose a look, or match your device settings.'),
            const SizedBox(height: 14),
            Wrap(
              spacing: 8,
              runSpacing: 8,
              children: [
                for (final option in [
                  (ThemeMode.light, 'Light', Icons.light_mode_outlined),
                  (ThemeMode.dark, 'Dark', Icons.dark_mode_outlined),
                  (
                    ThemeMode.system,
                    'System',
                    Icons.settings_brightness_outlined,
                  ),
                ])
                  ChoiceChip(
                    label: Text(option.$2),
                    avatar: Icon(option.$3, size: 18),
                    selected: controller.mode == option.$1,
                    onSelected: (_) async {
                      final saved = await controller.select(option.$1);
                      if (!saved && context.mounted) {
                        ScaffoldMessenger.of(context).showSnackBar(
                          const SnackBar(
                            content: Text(
                              'Appearance changed, but could not be saved. Please try again.',
                            ),
                          ),
                        );
                      }
                    },
                  ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}
