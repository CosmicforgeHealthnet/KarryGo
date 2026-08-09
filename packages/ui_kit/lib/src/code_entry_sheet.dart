import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:mobile_scanner/mobile_scanner.dart';

import '../cosmicforge_logistics_ui_kit.dart';

/// A bottom sheet that lets the user provide a handover verification code
/// either by scanning a QR code or typing it manually. Returns the resolved
/// code string via `Navigator.pop(context, code)`, or null if dismissed.
///
/// Show it with:
/// ```dart
/// final code = await showModalBottomSheet<String>(
///   context: context,
///   isScrollControlled: true,
///   builder: (_) => CosmicforgeLogisticsCodeEntrySheet(title: '...'),
/// );
/// ```
class CosmicforgeLogisticsCodeEntrySheet extends StatefulWidget {
  const CosmicforgeLogisticsCodeEntrySheet({
    super.key,
    required this.title,
    this.subtitle,
    this.codeLength = 6,
  });

  final String title;
  final String? subtitle;
  final int codeLength;

  @override
  State<CosmicforgeLogisticsCodeEntrySheet> createState() =>
      _CosmicforgeLogisticsCodeEntrySheetState();
}

class _CosmicforgeLogisticsCodeEntrySheetState
    extends State<CosmicforgeLogisticsCodeEntrySheet> {
  bool _scanMode = true;
  final _manualController = TextEditingController();
  MobileScannerController? _scannerController;
  bool _handledScan = false;

  @override
  void dispose() {
    _manualController.dispose();
    _scannerController?.dispose();
    super.dispose();
  }

  void _onDetect(BarcodeCapture capture) {
    if (_handledScan) return;
    for (final barcode in capture.barcodes) {
      final raw = barcode.rawValue;
      if (raw == null) continue;
      final digits = raw.replaceAll(RegExp(r'\D'), '');
      if (digits.isNotEmpty) {
        _handledScan = true;
        Navigator.of(context).pop(digits);
        return;
      }
    }
  }

  void _submitManual() {
    final code = _manualController.text.trim();
    if (code.isEmpty) return;
    Navigator.of(context).pop(code);
  }

  @override
  Widget build(BuildContext context) {
    return SafeArea(
      child: Padding(
        padding: EdgeInsets.only(
          left: CosmicforgeLogisticsSpacing.md,
          right: CosmicforgeLogisticsSpacing.md,
          top: CosmicforgeLogisticsSpacing.md,
          bottom: MediaQuery.of(context).viewInsets.bottom +
              CosmicforgeLogisticsSpacing.md,
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Center(
              child: Container(
                width: 40,
                height: 4,
                decoration: BoxDecoration(
                  color: CosmicforgeLogisticsColors.border,
                  borderRadius: BorderRadius.circular(2),
                ),
              ),
            ),
            const SizedBox(height: CosmicforgeLogisticsSpacing.md),
            Text(
              widget.title,
              textAlign: TextAlign.center,
              style: const TextStyle(
                fontSize: 16,
                fontWeight: FontWeight.w800,
                color: CosmicforgeLogisticsColors.text,
              ),
            ),
            if (widget.subtitle != null) ...[
              const SizedBox(height: CosmicforgeLogisticsSpacing.xxs),
              Text(
                widget.subtitle!,
                textAlign: TextAlign.center,
                style: const TextStyle(
                  fontSize: 13,
                  color: CosmicforgeLogisticsColors.textMuted,
                ),
              ),
            ],
            const SizedBox(height: CosmicforgeLogisticsSpacing.md),
            _ModeToggle(
              scanMode: _scanMode,
              onChanged: (value) => setState(() {
                _scanMode = value;
                _handledScan = false;
              }),
            ),
            const SizedBox(height: CosmicforgeLogisticsSpacing.md),
            if (_scanMode)
              _ScannerView(
                onDetect: _onDetect,
                onControllerReady: (c) => _scannerController = c,
              )
            else
              _ManualEntry(
                controller: _manualController,
                codeLength: widget.codeLength,
                onSubmit: _submitManual,
              ),
          ],
        ),
      ),
    );
  }
}

class _ModeToggle extends StatelessWidget {
  const _ModeToggle({required this.scanMode, required this.onChanged});

  final bool scanMode;
  final ValueChanged<bool> onChanged;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(4),
      decoration: BoxDecoration(
        color: CosmicforgeLogisticsColors.surface,
        borderRadius: BorderRadius.circular(12),
      ),
      child: Row(
        children: [
          Expanded(
            child: _ModeButton(
              label: 'Scan QR',
              selected: scanMode,
              onTap: () => onChanged(true),
            ),
          ),
          Expanded(
            child: _ModeButton(
              label: 'Enter manually',
              selected: !scanMode,
              onTap: () => onChanged(false),
            ),
          ),
        ],
      ),
    );
  }
}

class _ModeButton extends StatelessWidget {
  const _ModeButton({
    required this.label,
    required this.selected,
    required this.onTap,
  });

  final String label;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 150),
        padding: const EdgeInsets.symmetric(vertical: 10),
        decoration: BoxDecoration(
          color: selected ? CosmicforgeLogisticsColors.card : null,
          borderRadius: BorderRadius.circular(10),
          boxShadow: selected
              ? const [
                  BoxShadow(
                    color: Color(0x14000000),
                    blurRadius: 6,
                    offset: Offset(0, 2),
                  ),
                ]
              : null,
        ),
        child: Text(
          label,
          textAlign: TextAlign.center,
          style: TextStyle(
            fontSize: 13,
            fontWeight: FontWeight.w700,
            color: selected
                ? CosmicforgeLogisticsColors.text
                : CosmicforgeLogisticsColors.textMuted,
          ),
        ),
      ),
    );
  }
}

class _ScannerView extends StatelessWidget {
  const _ScannerView({required this.onDetect, required this.onControllerReady});

  final void Function(BarcodeCapture) onDetect;
  final void Function(MobileScannerController) onControllerReady;

  @override
  Widget build(BuildContext context) {
    final controller = MobileScannerController();
    onControllerReady(controller);
    return ClipRRect(
      borderRadius: BorderRadius.circular(16),
      child: SizedBox(
        height: 280,
        child: MobileScanner(controller: controller, onDetect: onDetect),
      ),
    );
  }
}

class _ManualEntry extends StatelessWidget {
  const _ManualEntry({
    required this.controller,
    required this.codeLength,
    required this.onSubmit,
  });

  final TextEditingController controller;
  final int codeLength;
  final VoidCallback onSubmit;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        TextField(
          controller: controller,
          autofocus: true,
          keyboardType: TextInputType.number,
          textAlign: TextAlign.center,
          maxLength: codeLength,
          inputFormatters: [FilteringTextInputFormatter.digitsOnly],
          style: const TextStyle(
            fontSize: 24,
            fontWeight: FontWeight.w800,
            letterSpacing: 8,
            color: CosmicforgeLogisticsColors.text,
          ),
          decoration: InputDecoration(
            counterText: '',
            hintText: '0' * codeLength,
            filled: true,
            fillColor: CosmicforgeLogisticsColors.surface,
            border: OutlineInputBorder(
              borderRadius: BorderRadius.circular(12),
              borderSide: BorderSide.none,
            ),
          ),
          onSubmitted: (_) => onSubmit(),
        ),
        const SizedBox(height: CosmicforgeLogisticsSpacing.md),
        CosmicforgeLogisticsButton(label: 'Confirm', onPressed: onSubmit),
      ],
    );
  }
}
