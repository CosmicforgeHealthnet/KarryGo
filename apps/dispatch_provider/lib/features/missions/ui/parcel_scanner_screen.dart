import 'package:flutter/material.dart';
import 'package:mobile_scanner/mobile_scanner.dart';

import '../../../core/theme/theme_extensions.dart';

class ParcelScannerScreen extends StatefulWidget {
  const ParcelScannerScreen({super.key});
  @override
  State<ParcelScannerScreen> createState() => _ParcelScannerScreenState();
}

class _ParcelScannerScreenState extends State<ParcelScannerScreen> {
  final _camera = MobileScannerController();
  final _manual = TextEditingController();
  bool _returned = false;
  void _finish(String? code) {
    if (_returned || code == null || code.trim().isEmpty) return;
    _returned = true;
    Navigator.of(context).pop(code.trim());
  }

  @override
  void dispose() {
    _camera.dispose();
    _manual.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => Scaffold(
    appBar: AppBar(title: const Text('Scan parcel')),
    body: SafeArea(
      child: Column(
        children: [
          Expanded(
            child: ClipRRect(
              borderRadius: BorderRadius.circular(24),
              child: MobileScanner(
                controller: _camera,
                onDetect: (capture) =>
                    _finish(capture.barcodes.firstOrNull?.rawValue),
                errorBuilder: (_, error, child) => Center(
                  child: Padding(
                    padding: const EdgeInsets.all(24),
                    child: Text(
                      'Camera unavailable. Allow camera access or enter the parcel code below.',
                      textAlign: TextAlign.center,
                      style: TextStyle(color: context.palette.text),
                    ),
                  ),
                ),
              ),
            ),
          ),
          Padding(
            padding: const EdgeInsets.all(20),
            child: Column(
              children: [
                TextField(
                  controller: _manual,
                  decoration: const InputDecoration(
                    labelText: 'Parcel code',
                    hintText: 'Use the code printed on the parcel',
                  ),
                  onSubmitted: _finish,
                ),
                const SizedBox(height: 12),
                SizedBox(
                  width: double.infinity,
                  child: FilledButton(
                    onPressed: () => _finish(_manual.text),
                    child: const Text('Use parcel code'),
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    ),
  );
}
