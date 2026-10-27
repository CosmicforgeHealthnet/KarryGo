import 'package:cosmicforge_logistics_ui_kit/cosmicforge_logistics_ui_kit.dart';
import 'package:flutter/material.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// Asks for the day's fuel price at most once a day: the first time the driver
/// goes online. Dismissing it counts, so "Not today" means not until tomorrow.
class FuelPromptSchedule {
  static const _key = 'fuel_prompt_day';

  static String _today(DateTime now) =>
      '${now.year}-${now.month.toString().padLeft(2, '0')}-${now.day.toString().padLeft(2, '0')}';

  /// Whether today's prompt is still due; marks it shown when it is. Storage
  /// failures never prompt, so a broken store cannot nag.
  static Future<bool> claimToday({DateTime? now}) async {
    try {
      final prefs = await SharedPreferences.getInstance();
      final today = _today(now ?? DateTime.now());
      if (prefs.getString(_key) == today) return false;
      await prefs.setString(_key, today);
      return true;
    } catch (_) {
      return false;
    }
  }
}

/// Opens the fuel price sheet. [report] sends the entry and returns an error
/// message or null.
Future<void> showFuelPriceSheet(
  BuildContext context, {
  required Future<String?> Function(FuelPriceEntry) report,
  bool prompted = false,
}) async {
  final sent = await FuelPriceSheet.show(
    context,
    onSubmit: report,
    initialFuel: FuelType.petrol,
    prompted: prompted,
  );
  if (sent && context.mounted) {
    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(content: Text('Thanks! Your fuel price was sent.')),
    );
  }
}
