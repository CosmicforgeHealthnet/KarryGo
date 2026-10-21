import 'dart:async';
import 'package:flutter/material.dart';
import '../../../core/theme/theme_extensions.dart';
import '../../../shared/widgets/dispatch_screen_frame.dart';
import '../../../shared/widgets/delivery_route_preview.dart';
import '../../../shared/widgets/async_content.dart';
import '../data/request_model.dart';
import '../state/requests_controller.dart';
import 'widgets/opportunity_card.dart';

class RequestDetailScreen extends StatefulWidget {
  const RequestDetailScreen({
    super.key,
    required this.request,
    required this.onAccept,
    required this.onDecline,
    required this.onBrowseNew,
    required this.onBackToOpportunities,
    this.controller,
  });
  final RequestModel request;
  final FutureOr<void> Function() onAccept, onDecline;
  final VoidCallback onBrowseNew, onBackToOpportunities;
  final RequestsController? controller;
  @override
  State<RequestDetailScreen> createState() => _RequestDetailScreenState();
}

class _RequestDetailScreenState extends State<RequestDetailScreen> {
  bool _busy = false;
  Timer? _expiry;
  RequestModel get request =>
      widget.controller?.requests
          .where(
            (r) => r.id == widget.request.id && r.kind == widget.request.kind,
          )
          .firstOrNull ??
      widget.request;
  @override
  void initState() {
    super.initState();
    widget.controller?.addListener(_changed);
    _armExpiry();
  }

  void _armExpiry() {
    _expiry?.cancel();
    final at = request.expiresAt;
    if (at != null && at.isAfter(DateTime.now()))
      _expiry = Timer(at.difference(DateTime.now()), () {
        if (mounted) setState(() {});
      });
  }

  void _changed() {
    if (mounted) {
      _armExpiry();
      setState(() {});
    }
  }

  @override
  void dispose() {
    _expiry?.cancel();
    widget.controller?.removeListener(_changed);
    super.dispose();
  }

  Future<void> _act(bool accept) async {
    if (_busy) return;
    setState(() => _busy = true);
    try {
      await (accept ? widget.onAccept() : widget.onDecline());
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final r = request,
        unavailable = !request.isAcceptable,
        degraded = widget.controller != null &&
            !widget.controller!.canAcceptOffers;
    return PopScope(
      canPop: !_busy,
      child: Scaffold(
        backgroundColor: context.palette.background,
        appBar: AppBar(
          leading: DispatchBackButton(
            onPressed: _busy ? () {} : () => Navigator.maybePop(context),
          ),
          title: const Text('Delivery request'),
        ),
        body: SingleChildScrollView(
          padding: const EdgeInsets.fromLTRB(16, 8, 16, 24),
          child: Center(
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 600),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  if (unavailable) ...[
                    ActionNotice(
                      message: r.isExpired
                          ? 'This request has expired.'
                          : 'This request is no longer available.',
                    ),
                    const SizedBox(height: 16),
                  ],
                  if (r.hasMap) ...[
                    DeliveryRoutePreview(
                      pickupLat: r.pickupLat!,
                      pickupLng: r.pickupLng!,
                      dropoffLat: r.dropoffLat!,
                      dropoffLng: r.dropoffLng!,
                    ),
                    const SizedBox(height: 16),
                  ],
                  if (degraded && !unavailable) ...[
                    const ActionNotice(
                      message: 'This opportunity has not been refreshed. '
                          'Return to Opportunities and retry before accepting.',
                      isError: true,
                    ),
                    const SizedBox(height: 16),
                  ],
                  OpportunityCard(
                    request: r,
                    busy: _busy,
                    actionsAvailable: !degraded,
                    detailed: true,
                    onTap: () {},
                    onAccept: () => _act(true),
                    onDecline: () => _act(false),
                  ),
                ],
              ),
            ),
          ),
        ),
        bottomNavigationBar: SafeArea(
          top: false,
          child: Container(
            padding: const EdgeInsets.fromLTRB(20, 12, 20, 16),
            decoration: BoxDecoration(
              color: context.palette.surface,
              border: Border(top: BorderSide(color: context.palette.divider)),
            ),
            child: unavailable || degraded
                ? FilledButton(
                    onPressed: widget.onBrowseNew,
                    child: const Text('Back to opportunities'),
                  )
                : Row(
                    children: [
                      Expanded(
                        child: OutlinedButton(
                          onPressed: _busy ? null : () => _act(false),
                          style: OutlinedButton.styleFrom(
                            foregroundColor: context.palette.danger,
                            side: BorderSide(
                              color: context.palette.danger.withValues(
                                alpha: .3,
                              ),
                            ),
                          ),
                          child: const Text('Decline'),
                        ),
                      ),
                      const SizedBox(width: 10),
                      Expanded(
                        child: FilledButton(
                          onPressed: _busy ? null : () => _act(true),
                          child: _busy
                              ? const SizedBox.square(
                                  dimension: 18,
                                  child: CircularProgressIndicator(
                                    strokeWidth: 2,
                                  ),
                                )
                              : Text(
                                  r.lifecycle == RequestLifecycleState.requoted
                                      ? 'Accept update'
                                      : 'Accept',
                                ),
                        ),
                      ),
                    ],
                  ),
          ),
        ),
      ),
    );
  }
}
