import 'package:flutter/material.dart';
import 'package:flutter_map/flutter_map.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:latlong2/latlong.dart';

import '../../availability/state/availability_controller.dart';
import '../../requests/data/request_model.dart';

export '../../requests/data/request_model.dart' show RequestModel;

enum TripState {
  offline,
  online,
  arriving,
  arrivedAtPickup,
  tripStarted,
  tripCompleted,
}

class HomeScreen extends StatefulWidget {
  const HomeScreen({
    super.key,
    required this.tripStateNotifier,
    required this.activeRequestNotifier,
    required this.availabilityController,
  });

  final ValueNotifier<TripState> tripStateNotifier;
  final ValueNotifier<RequestModel?> activeRequestNotifier;
  final AvailabilityController availabilityController;

  @override
  HomeScreenState createState() => HomeScreenState();
}

class HomeScreenState extends State<HomeScreen> {
  static const _center = LatLng(6.5355, 3.2910);

  TripState get _tripState => widget.tripStateNotifier.value;

  @override
  void initState() {
    super.initState();
    widget.tripStateNotifier.addListener(_onNotifierChanged);
    widget.activeRequestNotifier.addListener(_onNotifierChanged);
    widget.availabilityController.addListener(_onNotifierChanged);
    _loadInitialAvailability();
  }

  @override
  void dispose() {
    widget.tripStateNotifier.removeListener(_onNotifierChanged);
    widget.activeRequestNotifier.removeListener(_onNotifierChanged);
    widget.availabilityController.removeListener(_onNotifierChanged);
    super.dispose();
  }

  void _onNotifierChanged() => setState(() {});

  void _update(
    TripState newState, {
    RequestModel? request,
    bool clearRequest = false,
  }) {
    if (clearRequest) widget.activeRequestNotifier.value = null;
    if (request != null) widget.activeRequestNotifier.value = request;
    widget.tripStateNotifier.value = newState;
  }

  Future<void> _loadInitialAvailability() async {
    final result = await widget.availabilityController.loadAvailability();
    if (!mounted) return;
    result.when(
      success: (avail) {
        if (avail.isOnline && _tripState == TripState.offline) {
          _update(TripState.online);
        }
        if (avail.isOffline && _tripState != TripState.offline) {
          _update(TripState.offline, clearRequest: true);
        }
      },
      failure: (_) {},
    );
  }

  Future<void> _goOnline() async {
    final messenger = ScaffoldMessenger.of(context);
    final result = await widget.availabilityController.goOnline();
    if (!mounted) return;
    result.when(
      success: (_) => _update(TripState.online),
      failure: (error) {
        final message = switch (error.code) {
          ApiErrorCode.forbidden =>
            'You cannot go online until your profile, verification, and vehicle are approved.',
          ApiErrorCode.network => 'Cannot connect to Cosmicforge Logistics server.',
          _ =>
            error.message.isNotEmpty
                ? error.message
                : 'Failed to go online. Please try again.',
        };
        messenger.showSnackBar(
          SnackBar(
            content: Text(message),
            backgroundColor: Colors.red.shade800,
            duration: const Duration(seconds: 5),
          ),
        );
      },
    );
  }

  Future<void> _goOffline() async {
    final messenger = ScaffoldMessenger.of(context);
    final result = await widget.availabilityController.goOffline();
    if (!mounted) return;
    result.when(
      success: (_) => _update(TripState.offline, clearRequest: true),
      failure: (error) {
        final message = error.code == ApiErrorCode.network
            ? 'Cannot connect to Cosmicforge Logistics server.'
            : error.message.isNotEmpty
            ? error.message
            : 'Failed to go offline. Please try again.';
        messenger.showSnackBar(
          SnackBar(
            content: Text(message),
            backgroundColor: Colors.red.shade800,
            duration: const Duration(seconds: 4),
          ),
        );
      },
    );
  }

  @override
  Widget build(BuildContext context) {
    return Stack(
      children: [
        FlutterMap(
          options: const MapOptions(initialCenter: _center, initialZoom: 14),
          children: [
            TileLayer(
              urlTemplate: 'https://tile.openstreetmap.org/{z}/{x}/{y}.png',
              userAgentPackageName:
                  'com.cosmicforge.logistics.dispatchprovider',
            ),
            if (_tripState == TripState.arriving ||
                _tripState == TripState.arrivedAtPickup ||
                _tripState == TripState.tripStarted)
              PolylineLayer(
                polylines: [
                  Polyline(
                    points: [
                      const LatLng(6.5420, 3.2850),
                      const LatLng(6.5390, 3.2870),
                      const LatLng(6.5370, 3.2900),
                      const LatLng(6.5355, 3.2910),
                      const LatLng(6.5330, 3.2940),
                      const LatLng(6.5300, 3.2980),
                    ],
                    color: _tripState == TripState.tripStarted
                        ? const Color(0xFF4CAF50)
                        : Colors.black54,
                    strokeWidth: 4,
                    pattern: _tripState == TripState.arriving
                        ? StrokePattern.dashed(segments: const [10, 6])
                        : StrokePattern.solid(),
                  ),
                ],
              ),
            MarkerLayer(
              markers: [
                if (_tripState == TripState.arriving ||
                    _tripState == TripState.arrivedAtPickup ||
                    _tripState == TripState.tripStarted)
                  Marker(
                    point: const LatLng(6.5420, 3.2850),
                    width: 80,
                    height: 30,
                    child: const _MapLabel(label: 'Pick up'),
                  ),
                if (_tripState == TripState.arriving ||
                    _tripState == TripState.arrivedAtPickup ||
                    _tripState == TripState.tripStarted)
                  Marker(
                    point: const LatLng(6.5300, 3.2980),
                    width: 96,
                    height: 30,
                    child: const _MapLabel(label: 'Destination'),
                  ),
              ],
            ),
          ],
        ),

        ValueListenableBuilder<TripState>(
          valueListenable: widget.tripStateNotifier,
          builder: (context, tripState, _) {
            final isChanging =
                widget.availabilityController.isChangingStatus ||
                widget.availabilityController.isLoading;
            return SafeArea(
              child: Padding(
                padding: const EdgeInsets.symmetric(
                  horizontal: 16,
                  vertical: 8,
                ),
                child: Row(
                  mainAxisAlignment: MainAxisAlignment.spaceBetween,
                  children: [
                    GestureDetector(
                      onTap: isChanging
                          ? null
                          : tripState == TripState.offline
                          ? _goOnline
                          : _goOffline,
                      child: Container(
                        padding: const EdgeInsets.symmetric(
                          horizontal: 14,
                          vertical: 10,
                        ),
                        decoration: BoxDecoration(
                          color: Colors.white,
                          borderRadius: BorderRadius.circular(999),
                          boxShadow: [
                            BoxShadow(
                              color: Colors.black.withValues(alpha: 0.12),
                              blurRadius: 10,
                              offset: const Offset(0, 2),
                            ),
                          ],
                        ),
                        child: Row(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            if (isChanging)
                              const SizedBox(
                                width: 16,
                                height: 16,
                                child: CircularProgressIndicator(
                                  strokeWidth: 2,
                                  color: Color(0xFF4CAF50),
                                ),
                              )
                            else
                              Text(
                                tripState == TripState.offline
                                    ? 'Go Online'
                                    : 'Go Offline',
                                style: const TextStyle(
                                  fontSize: 14,
                                  fontWeight: FontWeight.w700,
                                  color: Color(0xFF1A1A1A),
                                ),
                              ),
                            const SizedBox(width: 10),
                            _ToggleSwitch(
                              isOn: tripState != TripState.offline,
                              disabled: isChanging,
                            ),
                          ],
                        ),
                      ),
                    ),
                    Container(
                      width: 44,
                      height: 44,
                      decoration: BoxDecoration(
                        color: Colors.white,
                        shape: BoxShape.circle,
                        boxShadow: [
                          BoxShadow(
                            color: Colors.black.withValues(alpha: 0.12),
                            blurRadius: 10,
                            offset: const Offset(0, 2),
                          ),
                        ],
                      ),
                      child: const Icon(
                        Icons.notifications_outlined,
                        size: 22,
                        color: Color(0xFF1A1A1A),
                      ),
                    ),
                  ],
                ),
              ),
            );
          },
        ),
      ],
    );
  }
}

class _MapLabel extends StatelessWidget {
  const _MapLabel({required this.label});
  final String label;

  @override
  Widget build(BuildContext context) {
    return Container(
      alignment: Alignment.center,
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
      decoration: BoxDecoration(
        color: Colors.black,
        borderRadius: BorderRadius.circular(20),
      ),
      child: Text(
        label,
        style: const TextStyle(color: Colors.white, fontSize: 11),
      ),
    );
  }
}

class _ToggleSwitch extends StatelessWidget {
  const _ToggleSwitch({required this.isOn, this.disabled = false});
  final bool isOn;
  final bool disabled;

  @override
  Widget build(BuildContext context) {
    final trackColor = disabled
        ? Colors.grey.shade300
        : isOn
        ? const Color(0xFF4CAF50)
        : const Color(0xFFDDDDDD);
    return AnimatedContainer(
      duration: const Duration(milliseconds: 200),
      width: 40,
      height: 22,
      decoration: BoxDecoration(
        color: trackColor,
        borderRadius: BorderRadius.circular(999),
      ),
      child: AnimatedAlign(
        duration: const Duration(milliseconds: 200),
        alignment: isOn ? Alignment.centerRight : Alignment.centerLeft,
        child: Container(
          margin: const EdgeInsets.all(3),
          width: 16,
          height: 16,
          decoration: const BoxDecoration(
            color: Colors.white,
            shape: BoxShape.circle,
          ),
        ),
      ),
    );
  }
}

// ── Offline Sheet ─────────────────────────────────────────────────────────────

class OfflineSheet extends StatelessWidget {
  const OfflineSheet({super.key, required this.onGoOnline});
  final VoidCallback onGoOnline;

  @override
  Widget build(BuildContext context) {
    return Container(
      margin: const EdgeInsets.fromLTRB(12, 0, 12, 12),
      padding: const EdgeInsets.fromLTRB(24, 28, 24, 28),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(24),
        boxShadow: [
          BoxShadow(
            color: Colors.black.withValues(alpha: 0.10),
            blurRadius: 20,
          ),
        ],
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Image.asset(
            'assets/figma/profile_submitted.png',
            width: 200,
            height: 180,
            fit: BoxFit.contain,
          ),
          const SizedBox(height: 20),
          const Text(
            'You are Offline!',
            style: TextStyle(
              fontSize: 20,
              fontWeight: FontWeight.w800,
              color: Color(0xFF1A1A1A),
            ),
          ),
          const SizedBox(height: 10),
          const Text(
            'Your account activity is currently set on offline, switch to online mode to start accepting trip requests.',
            textAlign: TextAlign.center,
            style: TextStyle(
              fontSize: 13,
              color: Color(0xFF888888),
              height: 1.5,
            ),
          ),
          const SizedBox(height: 24),
          SizedBox(
            width: double.infinity,
            height: 52,
            child: FilledButton(
              onPressed: onGoOnline,
              style: FilledButton.styleFrom(
                backgroundColor: const Color(0xFF4CAF50),
                shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(999),
                ),
              ),
              child: const Text(
                'Go Online',
                style: TextStyle(fontSize: 16, fontWeight: FontWeight.w700),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

// ── Incoming Requests Sheet ───────────────────────────────────────────────────

class IncomingRequestsSheet extends StatelessWidget {
  const IncomingRequestsSheet({
    super.key,
    required this.requests,
    required this.onAccept,
    required this.onReject,
    this.isLoading = false,
  });
  final List<RequestModel> requests;
  final ValueChanged<RequestModel> onAccept;
  final ValueChanged<RequestModel> onReject;
  final bool isLoading;

  @override
  Widget build(BuildContext context) {
    final bottomPad = MediaQuery.of(context).padding.bottom;
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const Padding(
          padding: EdgeInsets.fromLTRB(20, 12, 20, 8),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                'Incoming Requests...',
                style: TextStyle(
                  fontSize: 18,
                  fontWeight: FontWeight.w800,
                  color: Color(0xFF1A1A1A),
                ),
              ),
              SizedBox(height: 4),
              Text(
                'Accept a request to start a trip.',
                style: TextStyle(fontSize: 13, color: Color(0xFF888888)),
              ),
            ],
          ),
        ),
        if (isLoading)
          const Padding(
            padding: EdgeInsets.symmetric(vertical: 32),
            child: Center(child: CircularProgressIndicator()),
          )
        else if (requests.isEmpty)
          const Padding(
            padding: EdgeInsets.symmetric(vertical: 32, horizontal: 20),
            child: Center(
              child: Text(
                'No active requests',
                style: TextStyle(fontSize: 14, color: Color(0xFF888888)),
              ),
            ),
          )
        else
          SizedBox(
            height: 320,
            child: PageView.builder(
              padEnds: false,
              controller: PageController(viewportFraction: 0.88),
              itemCount: requests.length,
              itemBuilder: (context, i) => Padding(
                padding: EdgeInsets.only(
                  left: i == 0 ? 20 : 8,
                  right: i == requests.length - 1 ? 20 : 8,
                ),
                child: RequestCard(
                  request: requests[i],
                  onAccept: () => onAccept(requests[i]),
                  onReject: () => onReject(requests[i]),
                ),
              ),
            ),
          ),
        SizedBox(height: 12 + bottomPad),
      ],
    );
  }
}

// ── Request Card ──────────────────────────────────────────────────────────────

class RequestCard extends StatelessWidget {
  const RequestCard({
    super.key,
    required this.request,
    required this.onAccept,
    required this.onReject,
  });
  final RequestModel request;
  final VoidCallback onAccept;
  final VoidCallback onReject;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: const Color(0xFFE0E0E0)),
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Container(
                width: 52,
                height: 52,
                decoration: const BoxDecoration(
                  color: Color(0xFFD0D0D0),
                  shape: BoxShape.circle,
                ),
                child: const Icon(Icons.person, size: 30, color: Colors.white),
              ),
              const SizedBox(width: 14),
              Expanded(
                child: Text(
                  request.customerName,
                  style: const TextStyle(
                    fontSize: 22,
                    fontWeight: FontWeight.w800,
                    color: Color(0xFF1A1A1A),
                    height: 1.1,
                  ),
                ),
              ),
            ],
          ),
          const SizedBox(height: 10),
          TripStats(request: request),
          const SizedBox(height: 14),
          const Divider(height: 1, color: Color(0xFFF0F0F0)),
          const SizedBox(height: 14),
          RouteInfo(
            pickup: request.pickupAddress,
            dropoff: request.dropoffAddress,
          ),
          const SizedBox(height: 16),
          Row(
            children: [
              Expanded(
                child: SizedBox(
                  height: 46,
                  child: OutlinedButton(
                    onPressed: onReject,
                    style: OutlinedButton.styleFrom(
                      foregroundColor: const Color(0xFFE53935),
                      side: BorderSide.none,
                      backgroundColor: const Color(0xFFFFEBEE),
                      shape: RoundedRectangleBorder(
                        borderRadius: BorderRadius.circular(999),
                      ),
                    ),
                    child: const Text(
                      'Reject Request',
                      style: TextStyle(
                        fontWeight: FontWeight.w700,
                        fontSize: 13,
                      ),
                    ),
                  ),
                ),
              ),
              const SizedBox(width: 10),
              Expanded(
                child: SizedBox(
                  height: 46,
                  child: FilledButton(
                    onPressed: onAccept,
                    style: FilledButton.styleFrom(
                      backgroundColor: const Color(0xFF4CAF50),
                      shape: RoundedRectangleBorder(
                        borderRadius: BorderRadius.circular(999),
                      ),
                    ),
                    child: const Text(
                      'Accept Request',
                      style: TextStyle(
                        fontWeight: FontWeight.w700,
                        fontSize: 13,
                      ),
                    ),
                  ),
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }
}

// ── Trip Sheet (arriving / arrived) ──────────────────────────────────────────

class TripSheet extends StatelessWidget {
  const TripSheet({
    super.key,
    required this.request,
    required this.title,
    required this.subtitle,
    required this.showCall,
    required this.primaryLabel,
    required this.primaryEnabled,
    required this.secondaryLabel,
    required this.onPrimary,
    required this.onSecondary,
    this.onSimulate,
  });
  final RequestModel request;
  final String title;
  final String subtitle;
  final bool showCall;
  final String primaryLabel;
  final bool primaryEnabled;
  final String secondaryLabel;
  final VoidCallback onPrimary;
  final VoidCallback onSecondary;
  final VoidCallback? onSimulate;

  @override
  Widget build(BuildContext context) {
    final bottomPad = MediaQuery.of(context).padding.bottom;
    return Container(
      margin: EdgeInsets.fromLTRB(12, 0, 12, 12 + bottomPad),
      padding: const EdgeInsets.fromLTRB(16, 20, 16, 16),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(24),
        boxShadow: [
          BoxShadow(
            color: Colors.black.withValues(alpha: 0.10),
            blurRadius: 20,
          ),
        ],
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            title,
            style: const TextStyle(
              fontSize: 18,
              fontWeight: FontWeight.w800,
              color: Color(0xFF1A1A1A),
            ),
          ),
          const SizedBox(height: 4),
          Text(
            subtitle,
            style: const TextStyle(fontSize: 13, color: Color(0xFF888888)),
          ),
          const SizedBox(height: 16),
          Container(
            padding: const EdgeInsets.all(14),
            decoration: BoxDecoration(
              borderRadius: BorderRadius.circular(16),
              border: Border.all(color: const Color(0xFFE0E0E0)),
            ),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Row(
                  children: [
                    Container(
                      width: 44,
                      height: 44,
                      decoration: const BoxDecoration(
                        color: Color(0xFFD0D0D0),
                        shape: BoxShape.circle,
                      ),
                      child: const Icon(
                        Icons.person,
                        size: 26,
                        color: Colors.white,
                      ),
                    ),
                    const SizedBox(width: 12),
                    Expanded(
                      child: Text(
                        request.customerName,
                        style: const TextStyle(
                          fontSize: 16,
                          fontWeight: FontWeight.w800,
                          color: Color(0xFF1A1A1A),
                        ),
                      ),
                    ),
                    if (showCall)
                      Column(
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          Container(
                            width: 44,
                            height: 44,
                            decoration: const BoxDecoration(
                              color: Color(0xFFE8F5E9),
                              shape: BoxShape.circle,
                            ),
                            child: const Icon(
                              Icons.phone,
                              color: Color(0xFF4CAF50),
                              size: 20,
                            ),
                          ),
                          const SizedBox(height: 4),
                          const Text(
                            'Call',
                            style: TextStyle(
                              fontSize: 11,
                              color: Color(0xFF888888),
                            ),
                          ),
                        ],
                      ),
                  ],
                ),
                const SizedBox(height: 8),
                TripStats(request: request),
                const SizedBox(height: 12),
                RouteInfo(
                  pickup: request.pickupAddress,
                  dropoff: request.dropoffAddress,
                ),
                const SizedBox(height: 16),
                Row(
                  children: [
                    Expanded(
                      child: SizedBox(
                        height: 46,
                        child: OutlinedButton(
                          onPressed: onSecondary,
                          style: OutlinedButton.styleFrom(
                            foregroundColor: const Color(0xFFE53935),
                            backgroundColor: const Color(0xFFFFEBEE),
                            side: BorderSide.none,
                            shape: RoundedRectangleBorder(
                              borderRadius: BorderRadius.circular(999),
                            ),
                          ),
                          child: Text(
                            secondaryLabel,
                            style: const TextStyle(fontWeight: FontWeight.w700),
                          ),
                        ),
                      ),
                    ),
                    const SizedBox(width: 10),
                    Expanded(
                      child: SizedBox(
                        height: 46,
                        child: FilledButton(
                          onPressed: primaryEnabled ? onPrimary : null,
                          style: FilledButton.styleFrom(
                            backgroundColor: const Color(0xFF4CAF50),
                            disabledBackgroundColor: const Color(
                              0xFF4CAF50,
                            ).withValues(alpha: 0.4),
                            shape: RoundedRectangleBorder(
                              borderRadius: BorderRadius.circular(999),
                            ),
                          ),
                          child: Text(
                            primaryLabel,
                            style: const TextStyle(fontWeight: FontWeight.w700),
                          ),
                        ),
                      ),
                    ),
                  ],
                ),
                if (onSimulate != null) ...[
                  const SizedBox(height: 8),
                  TextButton(
                    onPressed: onSimulate,
                    child: const Text(
                      'Simulate arrival (dev only)',
                      style: TextStyle(fontSize: 11),
                    ),
                  ),
                ],
              ],
            ),
          ),
        ],
      ),
    );
  }
}

// ── Trip Started Sheet ────────────────────────────────────────────────────────

class TripStartedSheet extends StatelessWidget {
  const TripStartedSheet({super.key, required this.request, required this.onEndTrip});
  final RequestModel request;
  final VoidCallback onEndTrip;

  @override
  Widget build(BuildContext context) {
    final bottomPad = MediaQuery.of(context).padding.bottom;
    return Container(
      margin: EdgeInsets.fromLTRB(12, 0, 12, 12 + bottomPad),
      padding: const EdgeInsets.fromLTRB(16, 20, 16, 16),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(24),
        boxShadow: [
          BoxShadow(
            color: Colors.black.withValues(alpha: 0.10),
            blurRadius: 20,
          ),
        ],
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text(
            'Trip has started...',
            style: TextStyle(
              fontSize: 18,
              fontWeight: FontWeight.w800,
              color: Color(0xFF1A1A1A),
            ),
          ),
          const SizedBox(height: 4),
          const Text(
            'You are on your way to deliver a package.',
            style: TextStyle(fontSize: 13, color: Color(0xFF888888)),
          ),
          const SizedBox(height: 16),
          Container(
            padding: const EdgeInsets.all(14),
            decoration: BoxDecoration(
              borderRadius: BorderRadius.circular(16),
              border: Border.all(color: const Color(0xFFE0E0E0)),
            ),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Row(
                  children: [
                    Container(
                      width: 44,
                      height: 44,
                      decoration: const BoxDecoration(
                        color: Color(0xFFD0D0D0),
                        shape: BoxShape.circle,
                      ),
                      child: const Icon(
                        Icons.person,
                        size: 26,
                        color: Colors.white,
                      ),
                    ),
                    const SizedBox(width: 12),
                    Expanded(
                      child: Text(
                        request.customerName,
                        style: const TextStyle(
                          fontSize: 16,
                          fontWeight: FontWeight.w800,
                        ),
                      ),
                    ),
                  ],
                ),
                const SizedBox(height: 8),
                TripStats(request: request),
                const SizedBox(height: 12),
                RouteInfo(
                  pickup: request.pickupAddress,
                  dropoff: request.dropoffAddress,
                ),
                const SizedBox(height: 16),
                TweenAnimationBuilder<double>(
                  tween: Tween(begin: 0.0, end: 1.0),
                  duration: const Duration(seconds: 2),
                  curve: Curves.easeInOut,
                  builder: (context, value, _) {
                    return Column(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        Stack(
                          children: [
                            Padding(
                              padding: const EdgeInsets.symmetric(vertical: 10),
                              child: Container(
                                height: 3,
                                color: const Color(0xFF4CAF50),
                              ),
                            ),
                            Align(
                              alignment: Alignment(-1.0 + (value * 2.0), 0),
                              child: const Icon(
                                Icons.delivery_dining,
                                color: Color(0xFF4CAF50),
                                size: 28,
                              ),
                            ),
                          ],
                        ),
                        const Row(
                          mainAxisAlignment: MainAxisAlignment.spaceBetween,
                          children: [
                            Text(
                              'Pickup',
                              style: TextStyle(
                                fontSize: 11,
                                color: Color(0xFF888888),
                              ),
                            ),
                            Text(
                              'Destination',
                              style: TextStyle(
                                fontSize: 11,
                                color: Color(0xFF888888),
                              ),
                            ),
                          ],
                        ),
                      ],
                    );
                  },
                ),
                const SizedBox(height: 12),
                SizedBox(
                  width: double.infinity,
                  height: 50,
                  child: OutlinedButton(
                    onPressed: onEndTrip,
                    style: OutlinedButton.styleFrom(
                      foregroundColor: const Color(0xFFE53935),
                      backgroundColor: const Color(0xFFFFEBEE),
                      side: BorderSide.none,
                      shape: RoundedRectangleBorder(
                        borderRadius: BorderRadius.circular(999),
                      ),
                    ),
                    child: const Text(
                      'End Trip',
                      style: TextStyle(
                        fontWeight: FontWeight.w700,
                        fontSize: 16,
                      ),
                    ),
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

// ── Shared widgets ────────────────────────────────────────────────────────────

class TripStats extends StatelessWidget {
  const TripStats({super.key, required this.request});
  final RequestModel request;

  @override
  Widget build(BuildContext context) {
    final tierLabel = request.serviceTierLabel ??
        (request.isExpress ? 'Express Delivery' : 'Standard Delivery');
    final tierColor =
        request.isExpress ? const Color(0xFFF57C00) : const Color(0xFF888888);

    return Row(
      children: [
        const Icon(Icons.av_timer, size: 14, color: Color(0xFF888888)),
        const SizedBox(width: 4),
        Text(
          request.distanceDisplay,
          style: const TextStyle(fontSize: 12, color: Color(0xFF888888)),
        ),
        const SizedBox(width: 12),
        const Icon(Icons.attach_money, size: 14, color: Color(0xFF888888)),
        const SizedBox(width: 4),
        Text(
          request.fareDisplay,
          style: const TextStyle(fontSize: 12, color: Color(0xFF888888)),
        ),
        const SizedBox(width: 12),
        Container(
          padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
          decoration: BoxDecoration(
            color: tierColor.withAlpha(26),
            borderRadius: BorderRadius.circular(4),
            border: Border.all(color: tierColor.withAlpha(77)),
          ),
          child: Text(
            tierLabel,
            style: TextStyle(
              fontSize: 10,
              fontWeight: FontWeight.w600,
              color: tierColor,
            ),
          ),
        ),
      ],
    );
  }
}

class RouteInfo extends StatelessWidget {
  const RouteInfo({super.key, required this.pickup, required this.dropoff});
  final String pickup;
  final String dropoff;

  @override
  Widget build(BuildContext context) {
    return IntrinsicHeight(
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          SizedBox(
            width: 18,
            child: Column(
              children: [
                Container(
                  width: 14,
                  height: 14,
                  decoration: const BoxDecoration(
                    color: Color(0xFF4CAF50),
                    shape: BoxShape.circle,
                  ),
                ),
                Expanded(
                  child: Center(
                    child: Container(width: 2, color: const Color(0xFF4CAF50)),
                  ),
                ),
                Container(
                  width: 14,
                  height: 14,
                  decoration: BoxDecoration(
                    shape: BoxShape.circle,
                    border: Border.all(
                      color: const Color(0xFF4CAF50),
                      width: 2.5,
                    ),
                  ),
                ),
              ],
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Text(
                  'Pick-up',
                  style: TextStyle(fontSize: 11, color: Color(0xFF888888)),
                ),
                const SizedBox(height: 2),
                Text(
                  pickup,
                  style: const TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w700,
                  ),
                ),
                const SizedBox(height: 14),
                const Text(
                  'Drop off',
                  style: TextStyle(fontSize: 11, color: Color(0xFF888888)),
                ),
                const SizedBox(height: 2),
                Text(
                  dropoff,
                  style: const TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w700,
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}
