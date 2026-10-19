import 'package:flutter/material.dart';
import 'package:geolocator/geolocator.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../../disputes/state/provider_dispute_controller.dart';
import 'provider_app_colors.dart';

/// Emergency options during a job: call 112, or alert Karryam support with
/// the driver's location (it goes to the top of their queue).
Future<void> showProviderSosSheet(
  BuildContext context, {
  required ProviderDisputeController support,
  required String bookingId,
}) {
  return showModalBottomSheet<void>(
    context: context,
    backgroundColor: Colors.white,
    shape: const RoundedRectangleBorder(borderRadius: BorderRadius.vertical(top: Radius.circular(24))),
    builder: (_) => _SosSheet(support: support, bookingId: bookingId),
  );
}

class _SosSheet extends StatefulWidget {
  const _SosSheet({required this.support, required this.bookingId});
  final ProviderDisputeController support;
  final String bookingId;

  @override
  State<_SosSheet> createState() => _SosSheetState();
}

class _SosSheetState extends State<_SosSheet> {
  static const _danger = Color(0xFFD32F2F);
  bool _sending = false;
  bool _sent = false;
  String? _error;

  Future<void> _alert() async {
    setState(() {
      _sending = true;
      _error = null;
    });
    double? lat, lng;
    try {
      if (await Geolocator.isLocationServiceEnabled()) {
        final pos = await Geolocator.getCurrentPosition(
          locationSettings: const LocationSettings(timeLimit: Duration(seconds: 8)),
        );
        lat = pos.latitude;
        lng = pos.longitude;
      }
    } catch (_) {
      // Send without a location rather than not at all.
    }
    final error = await widget.support.sendSos(
      description: 'Driver SOS during job ${widget.bookingId}',
      lat: lat,
      lng: lng,
    );
    if (!mounted) return;
    setState(() {
      _sending = false;
      _sent = error == null;
      _error = error;
    });
  }

  @override
  Widget build(BuildContext context) {
    return SafeArea(
      child: Padding(
        padding: const EdgeInsets.fromLTRB(24, 20, 24, 16),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            const Icon(Icons.sos_rounded, color: _danger, size: 44),
            const SizedBox(height: 8),
            const Text(
              'Are you in danger?',
              textAlign: TextAlign.center,
              style: TextStyle(color: kProviderText, fontSize: 18, fontWeight: FontWeight.w800),
            ),
            const SizedBox(height: 18),
            SizedBox(
              height: 52,
              child: FilledButton(
                onPressed: () => launchUrl(Uri(scheme: 'tel', path: '112')),
                style: FilledButton.styleFrom(
                  backgroundColor: _danger,
                  shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(30)),
                ),
                child: const Text('Call emergency services (112)', style: TextStyle(fontWeight: FontWeight.w700)),
              ),
            ),
            const SizedBox(height: 10),
            if (_sent)
              const Text(
                'Karryam support has your alert and location. Stay on the line with 112 if you can.',
                textAlign: TextAlign.center,
                style: TextStyle(color: kProviderText, fontSize: 13),
              )
            else
              OutlinedButton(
                onPressed: _sending ? null : _alert,
                style: OutlinedButton.styleFrom(
                  minimumSize: const Size.fromHeight(48),
                  shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(30)),
                ),
                child: Text(_sending ? 'Sending alert…' : 'Alert Karryam support',
                    style: const TextStyle(color: kProviderText)),
              ),
            if (_error != null) ...[
              const SizedBox(height: 8),
              Text(_error!, textAlign: TextAlign.center, style: const TextStyle(color: _danger, fontSize: 12)),
            ],
            TextButton(
              onPressed: () => Navigator.pop(context),
              child: const Text('Close', style: TextStyle(color: kProviderMuted)),
            ),
          ],
        ),
      ),
    );
  }
}
