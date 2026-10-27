import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

/// Fuel a driver can report.
enum FuelType {
  petrol('PETROL', 'Petrol'),
  diesel('DIESEL', 'Diesel'),
  gas('GAS', 'Gas');

  const FuelType(this.wire, this.label);
  final String wire;
  final String label;
}

/// What the driver entered.
class FuelPriceEntry {
  const FuelPriceEntry({required this.fuelType, required this.pricePerLitreKobo});
  final FuelType fuelType;
  final int pricePerLitreKobo;
}

/// Asks a driver what they paid for fuel today. [onSubmit] sends it and
/// returns an error message, or null on success. The sheet pops `true` once
/// sent and `null` when dismissed with "Not today".
///
/// It follows the app's [ThemeData], so it fits each driver app in light and
/// dark mode.
class FuelPriceSheet extends StatefulWidget {
  const FuelPriceSheet({
    super.key,
    required this.onSubmit,
    this.initialFuel = FuelType.petrol,
    this.prompted = false,
  });

  final Future<String?> Function(FuelPriceEntry entry) onSubmit;
  final FuelType initialFuel;

  /// True when the app opened it by itself (daily prompt) rather than from a
  /// menu; it then explains why it is asking.
  final bool prompted;

  /// Shows the sheet; returns true when a price was sent.
  static Future<bool> show(
    BuildContext context, {
    required Future<String?> Function(FuelPriceEntry entry) onSubmit,
    FuelType initialFuel = FuelType.petrol,
    bool prompted = false,
  }) async {
    final sent = await showModalBottomSheet<bool>(
      context: context,
      isScrollControlled: true,
      showDragHandle: true,
      builder: (_) => FuelPriceSheet(onSubmit: onSubmit, initialFuel: initialFuel, prompted: prompted),
    );
    return sent == true;
  }

  @override
  State<FuelPriceSheet> createState() => _FuelPriceSheetState();
}

class _FuelPriceSheetState extends State<FuelPriceSheet> {
  late FuelType _fuel = widget.initialFuel;
  final _price = TextEditingController();
  bool _busy = false;
  String? _error;

  @override
  void dispose() {
    _price.dispose();
    super.dispose();
  }

  int? get _priceKobo {
    final naira = int.tryParse(_price.text.replaceAll(',', '').trim());
    return naira == null ? null : naira * 100;
  }

  bool get _valid {
    final kobo = _priceKobo;
    return kobo != null && kobo >= 10000 && kobo <= 1000000;
  }

  Future<void> _submit() async {
    final kobo = _priceKobo;
    if (!_valid || kobo == null || _busy) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    final error = await widget.onSubmit(FuelPriceEntry(fuelType: _fuel, pricePerLitreKobo: kobo));
    if (!mounted) return;
    if (error == null) {
      Navigator.of(context).pop(true);
      return;
    }
    setState(() {
      _busy = false;
      _error = error;
    });
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final scheme = theme.colorScheme;
    final muted = scheme.onSurface.withValues(alpha: 0.65);
    final showRangeHint = _price.text.isNotEmpty && !_valid;
    return SafeArea(
      child: Padding(
        padding: EdgeInsets.fromLTRB(20, 0, 20, MediaQuery.of(context).viewInsets.bottom + 16),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Row(
              children: [
                Icon(Icons.local_gas_station_rounded, color: scheme.primary),
                const SizedBox(width: 10),
                Expanded(
                  child: Text(
                    'What did you pay for fuel today?',
                    style: theme.textTheme.titleMedium?.copyWith(fontWeight: FontWeight.w700),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 6),
            Text(
              widget.prompted
                  ? 'Your price helps keep fares fair in your area. It takes a few seconds, and you can skip it.'
                  : 'Your price helps keep fares fair in your area.',
              style: TextStyle(color: muted, height: 1.4),
            ),
            const SizedBox(height: 16),
            Wrap(
              spacing: 8,
              children: [
                for (final fuel in FuelType.values)
                  ChoiceChip(
                    label: Text(fuel.label),
                    selected: _fuel == fuel,
                    onSelected: _busy ? null : (_) => setState(() => _fuel = fuel),
                  ),
              ],
            ),
            const SizedBox(height: 14),
            TextField(
              controller: _price,
              enabled: !_busy,
              autofocus: !widget.prompted,
              keyboardType: TextInputType.number,
              inputFormatters: [FilteringTextInputFormatter.digitsOnly, LengthLimitingTextInputFormatter(5)],
              onChanged: (_) => setState(() => _error = null),
              onSubmitted: (_) => _submit(),
              style: const TextStyle(fontSize: 22, fontWeight: FontWeight.w700),
              decoration: InputDecoration(
                labelText: 'Price per litre',
                prefixText: '₦ ',
                suffixText: '/ litre',
                errorText: showRangeHint ? 'Enter between ₦100 and ₦10,000' : _error,
                border: OutlineInputBorder(borderRadius: BorderRadius.circular(12)),
              ),
            ),
            const SizedBox(height: 16),
            FilledButton(
              onPressed: _valid && !_busy ? _submit : null,
              style: FilledButton.styleFrom(minimumSize: const Size.fromHeight(50)),
              child: _busy
                  ? const SizedBox(width: 20, height: 20, child: CircularProgressIndicator(strokeWidth: 2))
                  : const Text('Send price', style: TextStyle(fontWeight: FontWeight.w700)),
            ),
            TextButton(
              onPressed: _busy ? null : () => Navigator.of(context).pop(),
              style: TextButton.styleFrom(minimumSize: const Size.fromHeight(44)),
              child: Text(widget.prompted ? 'Not today' : 'Cancel'),
            ),
          ],
        ),
      ),
    );
  }
}
