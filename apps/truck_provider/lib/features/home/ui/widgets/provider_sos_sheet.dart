import 'package:flutter/material.dart';
import 'package:geolocator/geolocator.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../../../core/widgets/karryam_ui.dart';
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
        padding: const EdgeInsets.fromLTRB(24, 4, 24, 16),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Center(
              child: Container(
                width: 64,
                height: 64,
                decoration: BoxDecoration(color: kProviderRejectBg, shape: BoxShape.circle),
                child: Icon(Icons.sos_rounded, color: kProviderRejectText, size: 32),
              ),
            ),
            const SizedBox(height: 14),
            Text(
              'Are you in danger?',
              textAlign: TextAlign.center,
              style: TextStyle(color: kProviderText, fontSize: 20, fontWeight: FontWeight.w700),
            ),
            const SizedBox(height: 6),
            Text(
              'Call emergency services first. You can also alert Karryam support with your location.',
              textAlign: TextAlign.center,
              style: TextStyle(color: kProviderMuted, fontSize: 13.5, height: 1.45),
            ),
            const SizedBox(height: 20),
            KPrimaryButton(
              label: 'Call emergency services (112)',
              icon: Icons.call_rounded,
              color: kProviderRejectText,
              onPressed: () => launchUrl(Uri(scheme: 'tel', path: '112')),
            ),
            const SizedBox(height: 10),
            if (_sent)
              const KNotice(
                'Karryam support has your alert and location. Stay on the line with 112 if you can.',
                tone: KTone.success,
              )
            else
              KSecondaryButton(
                label: _sending ? 'Sending alert…' : 'Alert Karryam support',
                icon: Icons.support_agent_rounded,
                loading: _sending,
                onPressed: _alert,
              ),
            if (_error != null) ...[
              const SizedBox(height: 10),
              KNotice(_error!, tone: KTone.danger),
            ],
            const SizedBox(height: 4),
            TextButton(
              onPressed: () => Navigator.pop(context),
              style: TextButton.styleFrom(foregroundColor: kProviderMuted),
              child: const Text('Close'),
            ),
          ],
        ),
      ),
    );
  }
}
