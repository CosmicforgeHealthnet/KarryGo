import 'dart:async';
import 'package:flutter/material.dart';
import '../../../app/vendor_runtime.dart';
import '../../../core/theme/vendor_theme.dart';
import '../../../core/widgets/vendor_widgets.dart';
import '../../onboarding/ui/business_onboarding_screen.dart';
import 'vendor_dashboard_screen.dart';

class VendorWorkspaceScreen extends StatefulWidget {
  const VendorWorkspaceScreen({
    super.key,
    required this.runtime,
    required this.onSettings,
  });
  final VendorRuntime runtime;
  final VoidCallback onSettings;
  @override
  State<VendorWorkspaceScreen> createState() => _VendorWorkspaceScreenState();
}

class _VendorWorkspaceScreenState extends State<VendorWorkspaceScreen> {
  bool? _active;
  bool _loading = true;
  String? _error;
  @override
  void initState() {
    super.initState();
    unawaited(_load());
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final app = widget.runtime.backend;
      final results = await Future.wait([
        app.businessApi.getBusinessesBusinessId(
          businessId: app.scope.vendorId!,
        ),
        app.onboardingApi.getOnboarding(),
      ]);
      if (mounted) {
        setState(
          () => _active =
              ({'ACTIVE', 'PAUSED'}.contains(results[0].object['status']) &&
                  results[1].object['status'] == 'ACTIVATED') ||
              ({
                    'PENDING_VERIFICATION',
                    'PENDING_REVIEW',
                    'ACTION_REQUIRED',
                    'IN_REVIEW',
                  }.contains(results[1].object['status']) &&
                  !{
                    'SUSPENDED',
                    'CLOSED',
                  }.contains(results[0].object['status'])),
        );
      }
    } catch (_) {
      if (mounted) {
        setState(
          () => _error = 'Your business could not be loaded. Try again.',
        );
      }
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_loading || _error != null) {
      return VendorScreenFrame(
        modeLabel: widget.runtime.mode.name,
        onSettings: widget.onSettings,
        body: Center(
          child: Padding(
            padding: const EdgeInsets.all(24),
            child: _loading
                ? CircularProgressIndicator(
                    color: KarryamColors.tealInk.forInk(context),
                  )
                : Column(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      InlineNotice(message: _error!, error: true),
                      const SizedBox(height: 20),
                      PrimaryAction(label: 'Try again', onPressed: _load),
                    ],
                  ),
          ),
        ),
      );
    }
    if (_active == true) {
      return VendorDashboardScreen(
        runtime: widget.runtime,
        onSettings: widget.onSettings,
      );
    }
    return BusinessOnboardingScreen(
      runtime: widget.runtime,
      onSettings: widget.onSettings,
      onActivated: () => setState(() => _active = true),
      onSubmitted: () => setState(() => _active = true),
    );
  }
}
