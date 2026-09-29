import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:mobile_scanner/mobile_scanner.dart';

import '../../../../core/theme/theme_extensions.dart';

/// Themed replacement for the UI-kit `CosmicforgeLogisticsCodeEntrySheet`.
///
/// Two tabs:
///   • Scan QR — real camera view, auto-detects a QR and returns its payload
///   • Enter manually — 6-digit code typed by the rider
///
/// Returns the scanned/typed code as a String, or null if the rider cancelled.
Future<String?> showCustomerVerificationSheet(BuildContext context) {
  return showModalBottomSheet<String>(
    context: context,
    isScrollControlled: true,
    useSafeArea: true,
    backgroundColor: Colors.transparent,
    barrierColor: Colors.black.withValues(alpha: 0.72),
    builder: (_) => const _CustomerVerificationSheet(),
  );
}

class _CustomerVerificationSheet extends StatefulWidget {
  const _CustomerVerificationSheet();

  @override
  State<_CustomerVerificationSheet> createState() =>
      _CustomerVerificationSheetState();
}

class _CustomerVerificationSheetState
    extends State<_CustomerVerificationSheet>
    with SingleTickerProviderStateMixin {
  static const int _codeLength = 6;

  late final TabController _tab;
  late final MobileScannerController _scanner;
  final _codeController = TextEditingController();
  final _focusNode = FocusNode();
  bool _handled = false;

  @override
  void initState() {
    super.initState();
    _tab = TabController(length: 2, vsync: this);
    _scanner = MobileScannerController(
      detectionSpeed: DetectionSpeed.noDuplicates,
      facing: CameraFacing.back,
      formats: const [BarcodeFormat.qrCode],
    );
    _tab.addListener(_onTabChanged);
    _codeController.addListener(_onCodeChanged);
    // Focus the manual input so the OS keyboard pops when they switch tabs.
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted && _tab.index == 1) _focusNode.requestFocus();
    });
  }

  void _onTabChanged() {
    setState(() {});
    if (_tab.index == 1) {
      // Manual tab: keyboard up, camera off.
      _focusNode.requestFocus();
    } else {
      // Scan tab: keyboard down, camera is auto-managed by the widget.
      _focusNode.unfocus();
    }
  }

  void _onCodeChanged() {
    setState(() {});
  }

  void _onDetect(BarcodeCapture capture) {
    if (_handled) return;
    final raw = capture.barcodes
        .map((b) => b.rawValue)
        .where((v) => v != null && v.isNotEmpty)
        .cast<String>()
        .firstOrNull;
    if (raw == null) return;
    _handled = true;
    Navigator.of(context).pop(raw.trim());
  }

  void _submitManual() {
    final code = _codeController.text.trim();
    if (code.length != _codeLength) return;
    Navigator.of(context).pop(code);
  }

  bool get _canSubmitManual =>
      _codeController.text.trim().length == _codeLength;

  @override
  void dispose() {
    _tab.removeListener(_onTabChanged);
    _tab.dispose();
    _scanner.dispose();
    _codeController.removeListener(_onCodeChanged);
    _codeController.dispose();
    _focusNode.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    final viewInsets = MediaQuery.of(context).viewInsets.bottom;

    return Padding(
      padding: EdgeInsets.only(bottom: viewInsets),
      child: Container(
        decoration: BoxDecoration(
          color: colors.surface,
          borderRadius: const BorderRadius.vertical(top: Radius.circular(28)),
          boxShadow: [
            BoxShadow(
              color: Colors.black.withValues(alpha: 0.22),
              blurRadius: 40,
              spreadRadius: -8,
              offset: const Offset(0, -6),
            ),
          ],
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            // ── Drag handle ───────────────────────────────────────
            Padding(
              padding: const EdgeInsets.only(top: 10, bottom: 14),
              child: Container(
                width: 40,
                height: 4,
                decoration: BoxDecoration(
                  color: colors.navy20,
                  borderRadius: BorderRadius.circular(2),
                ),
              ),
            ),

            // ── Title block ───────────────────────────────────────
            Padding(
              padding: const EdgeInsets.fromLTRB(24, 0, 24, 0),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    'Verify the customer',
                    style: TextStyle(
                      fontSize: 22,
                      fontWeight: FontWeight.w800,
                      color: colors.text,
                      letterSpacing: -0.6,
                      height: 1.15,
                    ),
                  ),
                  const SizedBox(height: 6),
                  Text(
                    "Scan the customer's QR or enter the code they read out.",
                    style: TextStyle(
                      fontSize: 13.5,
                      fontWeight: FontWeight.w400,
                      color: colors.textMuted,
                      height: 1.45,
                    ),
                  ),
                ],
              ),
            ),

            const SizedBox(height: 18),

            // ── Tab pills ─────────────────────────────────────────
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 24),
              child: Container(
                padding: const EdgeInsets.all(4),
                decoration: BoxDecoration(
                  color: colors.navy10,
                  borderRadius: BorderRadius.circular(14),
                ),
                child: Row(
                  children: [
                    Expanded(
                      child: _TabPill(
                        label: 'Scan QR',
                        icon: Icons.qr_code_scanner_rounded,
                        active: _tab.index == 0,
                        onTap: () => _tab.animateTo(0),
                      ),
                    ),
                    Expanded(
                      child: _TabPill(
                        label: 'Enter manually',
                        icon: Icons.keyboard_alt_outlined,
                        active: _tab.index == 1,
                        onTap: () => _tab.animateTo(1),
                      ),
                    ),
                  ],
                ),
              ),
            ),

            const SizedBox(height: 20),

            // ── Tab content ───────────────────────────────────────
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 24),
              child: AnimatedSize(
                duration: const Duration(milliseconds: 220),
                curve: Curves.easeOutCubic,
                child: _tab.index == 0
                    ? _ScanTab(
                        scanner: _scanner,
                        onDetect: _onDetect,
                      )
                    : _ManualTab(
                        controller: _codeController,
                        focusNode: _focusNode,
                        length: _codeLength,
                        onSubmitted: _canSubmitManual
                            ? _submitManual
                            : null,
                      ),
              ),
            ),

            const SizedBox(height: 22),

            // ── Action row ────────────────────────────────────────
            Padding(
              padding: const EdgeInsets.fromLTRB(24, 0, 24, 16),
              child: Row(
                children: [
                  Expanded(
                    child: SizedBox(
                      height: 52,
                      child: OutlinedButton(
                        onPressed: () => Navigator.of(context).pop(),
                        style: OutlinedButton.styleFrom(
                          foregroundColor: colors.text,
                          side: BorderSide(color: colors.border),
                          shape: RoundedRectangleBorder(
                            borderRadius: BorderRadius.circular(16),
                          ),
                          textStyle: const TextStyle(
                            fontSize: 15,
                            fontWeight: FontWeight.w700,
                          ),
                        ),
                        child: const Text('Cancel'),
                      ),
                    ),
                  ),
                  if (_tab.index == 1) ...[
                    const SizedBox(width: 12),
                    Expanded(
                      child: SizedBox(
                        height: 52,
                        child: FilledButton(
                          onPressed:
                              _canSubmitManual ? _submitManual : null,
                          style: FilledButton.styleFrom(
                            shape: RoundedRectangleBorder(
                              borderRadius: BorderRadius.circular(16),
                            ),
                            textStyle: const TextStyle(
                              fontSize: 15,
                              fontWeight: FontWeight.w700,
                            ),
                          ),
                          child: const Text('Verify'),
                        ),
                      ),
                    ),
                  ],
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ── Tab pill ────────────────────────────────────────────────────────────

class _TabPill extends StatelessWidget {
  const _TabPill({
    required this.label,
    required this.icon,
    required this.active,
    required this.onTap,
  });

  final String label;
  final IconData icon;
  final bool active;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    final fg = active ? colors.text : colors.textMuted;

    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 180),
        curve: Curves.easeOut,
        padding: const EdgeInsets.symmetric(vertical: 10),
        decoration: BoxDecoration(
          color: active ? colors.surface : Colors.transparent,
          borderRadius: BorderRadius.circular(10),
          boxShadow: active
              ? [
                  BoxShadow(
                    color: Colors.black.withValues(alpha: 0.06),
                    blurRadius: 6,
                    offset: const Offset(0, 2),
                  ),
                ]
              : null,
        ),
        child: Row(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(icon, size: 16, color: fg),
            const SizedBox(width: 6),
            Text(
              label,
              style: TextStyle(
                fontSize: 13,
                fontWeight: FontWeight.w700,
                color: fg,
                letterSpacing: -0.1,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ── Scan tab ────────────────────────────────────────────────────────────

class _ScanTab extends StatelessWidget {
  const _ScanTab({required this.scanner, required this.onDetect});

  final MobileScannerController scanner;
  final void Function(BarcodeCapture) onDetect;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;

    return Column(
      children: [
        // Camera view with QR frame overlay
        AspectRatio(
          aspectRatio: 1,
          child: ClipRRect(
            borderRadius: BorderRadius.circular(20),
            child: Stack(
              children: [
                Positioned.fill(
                  child: MobileScanner(
                    controller: scanner,
                    onDetect: onDetect,
                    fit: BoxFit.cover,
                  ),
                ),
                // Dark scrim with a clear window
                Positioned.fill(
                  child: CustomPaint(
                    painter: _QrFramePainter(
                      windowSize: 200,
                      scrim: Colors.black.withValues(alpha: 0.45),
                      corner: colors.brandTeal500,
                      cornerLength: 32,
                      cornerWidth: 4,
                      cornerRadius: 18,
                    ),
                  ),
                ),
                // Hint at bottom
                Positioned(
                  left: 0,
                  right: 0,
                  bottom: 16,
                  child: Center(
                    child: Container(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 14,
                        vertical: 8,
                      ),
                      decoration: BoxDecoration(
                        color: Colors.black.withValues(alpha: 0.55),
                        borderRadius: BorderRadius.circular(999),
                      ),
                      child: const Text(
                        "Point at the customer's QR code",
                        style: TextStyle(
                          fontSize: 12.5,
                          fontWeight: FontWeight.w600,
                          color: Colors.white,
                        ),
                      ),
                    ),
                  ),
                ),
              ],
            ),
          ),
        ),
      ],
    );
  }
}

class _QrFramePainter extends CustomPainter {
  _QrFramePainter({
    required this.windowSize,
    required this.scrim,
    required this.corner,
    required this.cornerLength,
    required this.cornerWidth,
    required this.cornerRadius,
  });

  final double windowSize;
  final Color scrim;
  final Color corner;
  final double cornerLength;
  final double cornerWidth;
  final double cornerRadius;

  @override
  void paint(Canvas canvas, Size size) {
    final rect = Rect.fromCenter(
      center: Offset(size.width / 2, size.height / 2),
      width: windowSize,
      height: windowSize,
    );

    // Dim everything outside the scanning window.
    final scrimPath = Path()
      ..addRect(Offset.zero & size)
      ..addRRect(
        RRect.fromRectAndRadius(rect, Radius.circular(cornerRadius)),
      )
      ..fillType = PathFillType.evenOdd;
    canvas.drawPath(scrimPath, Paint()..color = scrim);

    // Draw the four corner accents.
    final paint = Paint()
      ..color = corner
      ..strokeWidth = cornerWidth
      ..strokeCap = StrokeCap.round
      ..style = PaintingStyle.stroke;

    // Top-left
    canvas.drawPath(
      Path()
        ..moveTo(rect.left, rect.top + cornerLength)
        ..lineTo(rect.left, rect.top + cornerRadius)
        ..arcToPoint(
          Offset(rect.left + cornerRadius, rect.top),
          radius: Radius.circular(cornerRadius),
        )
        ..lineTo(rect.left + cornerLength, rect.top),
      paint,
    );
    // Top-right
    canvas.drawPath(
      Path()
        ..moveTo(rect.right - cornerLength, rect.top)
        ..lineTo(rect.right - cornerRadius, rect.top)
        ..arcToPoint(
          Offset(rect.right, rect.top + cornerRadius),
          radius: Radius.circular(cornerRadius),
        )
        ..lineTo(rect.right, rect.top + cornerLength),
      paint,
    );
    // Bottom-right
    canvas.drawPath(
      Path()
        ..moveTo(rect.right, rect.bottom - cornerLength)
        ..lineTo(rect.right, rect.bottom - cornerRadius)
        ..arcToPoint(
          Offset(rect.right - cornerRadius, rect.bottom),
          radius: Radius.circular(cornerRadius),
        )
        ..lineTo(rect.right - cornerLength, rect.bottom),
      paint,
    );
    // Bottom-left
    canvas.drawPath(
      Path()
        ..moveTo(rect.left + cornerLength, rect.bottom)
        ..lineTo(rect.left + cornerRadius, rect.bottom)
        ..arcToPoint(
          Offset(rect.left, rect.bottom - cornerRadius),
          radius: Radius.circular(cornerRadius),
        )
        ..lineTo(rect.left, rect.bottom - cornerLength),
      paint,
    );
  }

  @override
  bool shouldRepaint(covariant _QrFramePainter old) =>
      old.windowSize != windowSize ||
      old.scrim != scrim ||
      old.corner != corner;
}

// ── Manual tab ──────────────────────────────────────────────────────────

class _ManualTab extends StatelessWidget {
  const _ManualTab({
    required this.controller,
    required this.focusNode,
    required this.length,
    required this.onSubmitted,
  });

  final TextEditingController controller;
  final FocusNode focusNode;
  final int length;
  final VoidCallback? onSubmitted;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    final text = controller.text;
    final filled = text.length.clamp(0, length);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        // Hidden text field that owns the actual input. Visible digit boxes
        // below reflect its value. Using a native text field keeps the OS
        // numeric keyboard, paste, and autofill working for free.
        Stack(
          alignment: Alignment.center,
          children: [
            Opacity(
              opacity: 0.0,
              child: TextField(
                controller: controller,
                focusNode: focusNode,
                keyboardType: TextInputType.number,
                inputFormatters: [
                  FilteringTextInputFormatter.digitsOnly,
                  LengthLimitingTextInputFormatter(length),
                ],
                maxLength: length,
                textInputAction: TextInputAction.done,
                onSubmitted: (_) => onSubmitted?.call(),
                enableInteractiveSelection: false,
                showCursor: false,
                decoration: const InputDecoration(
                  counterText: '',
                  border: InputBorder.none,
                ),
              ),
            ),
            GestureDetector(
              onTap: () => focusNode.requestFocus(),
              behavior: HitTestBehavior.opaque,
              child: Row(
                children: [
                  for (int i = 0; i < length; i++) ...[
                    Expanded(
                      child: _CodeBox(
                        char: i < text.length ? text[i] : '',
                        focused: focusNode.hasFocus && i == filled,
                      ),
                    ),
                    if (i != length - 1) const SizedBox(width: 8),
                  ],
                ],
              ),
            ),
          ],
        ),
        const SizedBox(height: 12),
        Text(
          'The customer reads this code out from their app.',
          style: TextStyle(
            fontSize: 12.5,
            color: colors.textMuted,
            height: 1.4,
          ),
        ),
      ],
    );
  }
}

class _CodeBox extends StatelessWidget {
  const _CodeBox({required this.char, required this.focused});
  final String char;
  final bool focused;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;

    return AnimatedContainer(
      duration: const Duration(milliseconds: 140),
      height: 56,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: colors.navy10,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(
          color: focused ? colors.brandTeal500 : colors.divider,
          width: focused ? 1.8 : 1.2,
        ),
      ),
      child: Text(
        char,
        style: TextStyle(
          fontSize: 22,
          fontWeight: FontWeight.w800,
          color: colors.text,
          height: 1.0,
          fontFeatures: const [FontFeature.tabularFigures()],
        ),
      ),
    );
  }
}