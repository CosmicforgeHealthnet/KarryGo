import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:mobile_scanner/mobile_scanner.dart';
import '../theme/karryam_theme.dart';

class DriverCodeSheet extends StatefulWidget {
  const DriverCodeSheet({
    super.key,
    required this.title,
    required this.subtitle,
  });
  final String title, subtitle;
  @override
  State<DriverCodeSheet> createState() => _DriverCodeSheetState();
}

class _DriverCodeSheetState extends State<DriverCodeSheet> {
  final _text = TextEditingController();
  final _scanner = MobileScannerController(autoStart: false);
  bool _manual = true;
  bool _submitted = false;
  String? _error;
  void _submit(String raw) {
    if (_submitted) return;
    final digits = raw.replaceAll(RegExp(r'\D'), '');
    if (digits.length != 6) {
      setState(() => _error = 'Enter a six-digit code.');
      return;
    }
    _submitted = true;
    Navigator.pop(context, digits);
  }

  Future<void> _mode(bool manual) async {
    setState(() {
      _manual = manual;
      _error = null;
    });
    try {
      if (manual) {
        await _scanner.stop();
      } else {
        await WidgetsBinding.instance.endOfFrame;
        if (!mounted || _manual) return;
        await _scanner.start();
      }
    } catch (_) {
      if (mounted) {
        setState(() => _error = 'Camera unavailable. Enter the code manually.');
      }
    }
  }

  @override
  void dispose() {
    _scanner.dispose();
    _text.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => SafeArea(
    child: SingleChildScrollView(
      padding: EdgeInsets.fromLTRB(
        20,
        8,
        20,
        MediaQuery.viewInsetsOf(context).bottom + 24,
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(
            widget.title,
            style: const TextStyle(fontSize: 24, fontWeight: FontWeight.w600),
          ),
          const SizedBox(height: 12),
          Text(widget.subtitle),
          const SizedBox(height: 20),
          SegmentedButton<bool>(
            segments: const [
              ButtonSegment(
                value: false,
                label: Text('Scan QR'),
                icon: Icon(Icons.qr_code_scanner),
              ),
              ButtonSegment(
                value: true,
                label: Text('Enter code'),
                icon: Icon(Icons.dialpad),
              ),
            ],
            selected: {_manual},
            onSelectionChanged: (value) => _mode(value.single),
          ),
          const SizedBox(height: 20),
          if (!_manual)
            ClipRRect(
              borderRadius: BorderRadius.circular(18),
              child: SizedBox(
                height: 240,
                child: MobileScanner(
                  controller: _scanner,
                  errorBuilder: (context, error, child) => const Center(
                    child: Text('Camera unavailable. Use Enter code.'),
                  ),
                  onDetect: (capture) {
                    for (final barcode in capture.barcodes) {
                      if (barcode.rawValue != null) {
                        _submit(barcode.rawValue!);
                        break;
                      }
                    }
                  },
                ),
              ),
            ),
          if (_manual) ...[
            TextField(
              controller: _text,
              keyboardType: TextInputType.number,
              maxLength: 6,
              textAlign: TextAlign.center,
              inputFormatters: [FilteringTextInputFormatter.digitsOnly],
              style: const TextStyle(fontSize: 26, letterSpacing: 8),
              decoration: const InputDecoration(
                labelText: 'Six-digit code',
                counterText: '',
              ),
              onSubmitted: _submit,
            ),
            const SizedBox(height: 20),
            FilledButton(
              onPressed: () => _submit(_text.text),
              child: const Text('Verify code'),
            ),
          ],
          if (_error != null)
            Padding(
              padding: const EdgeInsets.only(top: 16),
              child: Text(
                _error!,
                style: TextStyle(color: KarryamColors.of(context).danger),
              ),
            ),
        ],
      ),
    ),
  );
}
