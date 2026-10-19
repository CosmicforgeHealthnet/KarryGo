import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/material.dart';
import 'package:geolocator/geolocator.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../../core/theme/karryam_theme.dart';
import '../../../core/widgets/karryam_ui.dart';
import '../data/support_api.dart';

/// Emergency options during a trip: call 112, or alert Karryam support with
/// the driver's location (it goes to the top of their queue).
Future<void> showSosSheet(
  BuildContext context, {
  required SupportApi api,
  required String tripId,
}) {
  return showModalBottomSheet<void>(
    context: context,
    backgroundColor: KarryamColors.of(context).card,
    builder: (sheetContext) => _SosSheet(api: api, tripId: tripId),
  );
}

class _SosSheet extends StatefulWidget {
  const _SosSheet({required this.api, required this.tripId});
  final SupportApi api;
  final String tripId;

  @override
  State<_SosSheet> createState() => _SosSheetState();
}

class _SosSheetState extends State<_SosSheet> {
  bool _sending = false;
  bool _sent = false;

  Future<void> _call() async {
    await launchUrl(Uri(scheme: 'tel', path: '112'));
  }

  Future<void> _alert() async {
    setState(() => _sending = true);
    double? lat, lng;
    try {
      if (!await Geolocator.isLocationServiceEnabled()) {
        throw const LocationServiceDisabledException();
      }
      final pos = await Geolocator.getCurrentPosition(
        locationSettings: const LocationSettings(
          timeLimit: Duration(seconds: 8),
        ),
      );
      lat = pos.latitude;
      lng = pos.longitude;
    } catch (_) {
      // Send the alert without a location rather than not at all.
    }
    try {
      await widget.api.sos(
        description: 'Driver SOS during trip ${widget.tripId}',
        lat: lat,
        lng: lng,
      );
      if (mounted) setState(() => _sent = true);
    } on ApiException catch (e) {
      if (mounted) showApiError(context, e);
    } finally {
      if (mounted) setState(() => _sending = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final colors = KarryamColors.of(context);
    return SafeArea(
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Icon(Icons.sos_rounded, size: 40, color: colors.danger),
            const SizedBox(height: 8),
            const Text(
              'Are you in danger?',
              textAlign: TextAlign.center,
              style: TextStyle(fontSize: 18, fontWeight: FontWeight.w800),
            ),
            const SizedBox(height: 16),
            KarryamButton(
              label: 'Call emergency services (112)',
              danger: true,
              onPressed: _call,
            ),
            const SizedBox(height: 10),
            if (_sent)
              KarryamPanel(
                child: Text(
                  'Karryam support has your alert and location. Stay on the line with 112 if you can.',
                  style: TextStyle(color: colors.text),
                ),
              )
            else
              OutlinedButton(
                onPressed: _sending ? null : _alert,
                child: Text(
                  _sending ? 'Sending alert…' : 'Alert Karryam support',
                ),
              ),
            TextButton(
              onPressed: () => Navigator.pop(context),
              child: const Text('Close'),
            ),
          ],
        ),
      ),
    );
  }
}
