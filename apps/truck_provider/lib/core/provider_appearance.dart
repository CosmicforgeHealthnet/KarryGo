import 'package:flutter/material.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// App appearance is independent of the authenticated session.
final providerAppearance = ProviderAppearance();

class ProviderAppearance extends ValueNotifier<ThemeMode> {
  ProviderAppearance() : super(ThemeMode.system);
  static const _key = 'truck_provider.appearance';
  bool _changed = false;

  Future<void> restore() async {
    try {
      final preferences = await SharedPreferences.getInstance();
      if (_changed) return;
      final saved = preferences.getString(_key);
      value =
          ThemeMode.values.where((mode) => mode.name == saved).firstOrNull ??
          ThemeMode.system;
    } catch (_) {
      // Appearance storage must never block sign-in.
    }
  }

  Future<void> select(ThemeMode mode) async {
    _changed = true;
    value = mode;
    try {
      final preferences = await SharedPreferences.getInstance();
      await preferences.setString(_key, mode.name);
    } catch (_) {
      // The chosen appearance still applies for this session.
    }
  }
}
