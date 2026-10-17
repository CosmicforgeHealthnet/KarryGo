import 'dart:math' as math;
import 'route_geometry.dart';
import 'driver_pin.dart';
import '../../../../shared/widgets/journey_icon.dart';
import 'package:flutter/material.dart';
import 'package:flutter_map/flutter_map.dart';
import 'package:latlong2/latlong.dart';

import '../../../../core/theme/theme_extensions.dart';
import '../../../availability/data/availability_models.dart';
import '../../../trips/data/trip_model.dart';

class LiveDispatchMap extends StatefulWidget {
  const LiveDispatchMap({
    super.key,
    this.location,
    this.demo = false,
    this.online = false,
    this.trip,
    required this.stale,
  });
  final bool demo;
  final bool online;
  final ProviderLocation? location;
  final TripModel? trip;
  final bool stale;
  @override
  State<LiveDispatchMap> createState() => _LiveDispatchMapState();
}

class _LiveDispatchMapState extends State<LiveDispatchMap>
    with TickerProviderStateMixin {
  final _map = MapController();
  late final AnimationController _cameraMotion;
  late final AnimationController _positionMotion;
  LatLng? _previousPosition;
  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _syncMotion();
  }

  void _syncMotion() {
    if (MediaQuery.disableAnimationsOf(context)) {
      _positionMotion.stop();
      _positionMotion.value = 1;
      return;
    }
    if (widget.demo &&
        widget.trip != null &&
        widget.trip!.statusCode != TripStatusCode.arrivedPickup) {
      _positionMotion.duration = const Duration(seconds: 24);
      _positionMotion.repeat(reverse: true);
    }
  }

  @override
  void didUpdateWidget(covariant LiveDispatchMap old) {
    super.didUpdateWidget(old);
    if (old.trip?.id != widget.trip?.id ||
        old.trip?.statusCode != widget.trip?.statusCode) {
      _positionMotion.stop();
      _positionMotion.value = 0;
      _syncMotion();
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (mounted && _ready) _fitTrip();
      });
    }
    if (!widget.demo &&
        widget.location != null &&
        (old.location?.lat != widget.location!.lat ||
            old.location?.lng != widget.location!.lng)) {
      _previousPosition = old.location == null
          ? null
          : LatLng(old.location!.lat, old.location!.lng);
      _positionMotion.duration = const Duration(milliseconds: 850);
      if (!MediaQuery.disableAnimationsOf(context))
        _positionMotion.forward(from: 0);
    }
  }

  void _fitTrip() {
    final t = widget.trip;
    if (t?.pickupLat == null ||
        t?.pickupLng == null ||
        t?.dropoffLat == null ||
        t?.dropoffLng == null)
      return;
    final points = [
      LatLng(t!.pickupLat!, t.pickupLng!),
      LatLng(t.dropoffLat!, t.dropoffLng!),
    ];
    if (!widget.demo && widget.location != null)
      points.add(LatLng(widget.location!.lat, widget.location!.lng));
    final height = _map.camera.nonRotatedSize.height;
    _map.fitCamera(
      CameraFit.coordinates(
        coordinates: points,
        padding: EdgeInsets.fromLTRB(
          70,
          math.min(200, height * .35),
          120,
          math.min(230, height * .4),
        ),
        maxZoom: 16,
      ),
    );
  }

  @override
  void initState() {
    super.initState();
    _positionMotion = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 850),
    );
    _cameraMotion = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 550),
    );
  }

  bool _ready = false;
  bool _tileError = false;
  int _tileGeneration = 0;
  void _recenter(LatLng point) {
    if (!_ready) return;
    if (MediaQuery.disableAnimationsOf(context)) {
      _map.rotate(0);
      _map.move(point, 15.5);
      return;
    }
    _cameraMotion.stop();
    final from = _map.camera.center;
    final zoom = _map.camera.zoom;
    final rotation = _map.camera.rotation;
    void frame() {
      final t = Curves.easeOutCubic.transform(_cameraMotion.value);
      _map.move(
        LatLng(
          from.latitude + (point.latitude - from.latitude) * t,
          from.longitude + (point.longitude - from.longitude) * t,
        ),
        zoom + (15.5 - zoom) * t,
      );
      _map.rotate(rotation * (1 - t));
    }

    _cameraMotion.addListener(frame);
    _cameraMotion
        .forward(from: 0)
        .whenCompleteOrCancel(() => _cameraMotion.removeListener(frame));
  }

  @override
  void dispose() {
    _positionMotion.dispose();
    _cameraMotion.dispose();
    _map.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final location = widget.location;
    final trip = widget.trip;
    final rider = widget.demo
        ? const LatLng(6.6018, 3.3515)
        : location == null
        ? null
        : LatLng(location.lat, location.lng);
    final pickup = trip?.pickupLat == null || trip?.pickupLng == null
        ? null
        : LatLng(trip!.pickupLat!, trip.pickupLng!);
    final dropoff = trip?.dropoffLat == null || trip?.dropoffLng == null
        ? null
        : LatLng(trip!.dropoffLat!, trip.dropoffLng!);
    final started =
        trip?.statusCode == TripStatusCode.inProgress ||
        trip?.statusCode == TripStatusCode.proofSubmitted;
    final route = decodeRoutePolyline(
      started ? trip?.routeDropoffPolyline : trip?.routePickupPolyline,
    );
    final demoStart = pickup == null
        ? null
        : LatLng(pickup.latitude + .004, pickup.longitude - .003);
    final demoPath = pickup == null || dropoff == null
        ? <LatLng>[]
        : started
        ? [pickup, LatLng(pickup.latitude, dropoff.longitude), dropoff]
        : [
            demoStart!,
            LatLng(pickup.latitude + .002, pickup.longitude),
            pickup,
          ];
    final visibleRoute = widget.demo ? demoPath : route;
    final center = rider ?? pickup ?? dropoff;
    if (center == null) {
      return Center(
        child: Padding(
          padding: const EdgeInsets.all(32),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const JourneyIcon(JourneySymbol.route, size: 64),
              const SizedBox(height: 16),
              Text(
                'Your route starts here',
                style: Theme.of(context).textTheme.titleLarge,
              ),
              const SizedBox(height: 8),
              Text(
                'Go online to share your location and see nearby opportunities.',
                textAlign: TextAlign.center,
                style: TextStyle(color: c.textMuted, height: 1.5),
              ),
            ],
          ),
        ),
      );
    }
    Marker marker(LatLng point, IconData icon, Color color, String label) =>
        Marker(
          point: point,
          width: 100,
          height: 62,
          child: Tooltip(
            message: label,
            child: Container(
              decoration: BoxDecoration(
                color: c.surface,
                borderRadius: BorderRadius.circular(16),
                border: Border.all(color: color, width: 2),
                boxShadow: c.cardShadow,
              ),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  JourneyIcon(
                    label == 'Pickup'
                        ? JourneySymbol.parcel
                        : JourneySymbol.route,
                    color: color,
                    size: 22,
                  ),
                  const SizedBox(height: 3),
                  Text(
                    label,
                    style: Theme.of(context).textTheme.bodySmall?.copyWith(
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                ],
              ),
            ),
          ),
        );
    return Stack(
      children: [
        FlutterMap(
          mapController: _map,
          options: MapOptions(
            initialCenter: center,
            initialZoom: 15.5,
            backgroundColor: c.surfaceElevated,
            onMapReady: () {
              _ready = true;
              if (widget.trip != null) _fitTrip();
            },
          ),
          children: [
            TileLayer(
              key: ValueKey(_tileGeneration),
              tileBuilder: context.isDark ? darkModeTileBuilder : null,
              urlTemplate: 'https://tile.openstreetmap.org/{z}/{x}/{y}.png',
              userAgentPackageName:
                  'com.cosmicforge.logistics.dispatchprovider',
              errorTileCallback: (_, __, ___) {
                if (mounted && !_tileError)
                  WidgetsBinding.instance.addPostFrameCallback((_) {
                    if (mounted) setState(() => _tileError = true);
                  });
              },
            ),
            if (visibleRoute.length > 1)
              PolylineLayer(
                polylines: [
                  Polyline(
                    points: visibleRoute,
                    color: c.surface,
                    strokeWidth: 9,
                  ),
                  Polyline(
                    points: visibleRoute,
                    color: c.brandTeal600,
                    strokeWidth: 5,
                  ),
                ],
              ),
            MarkerLayer(
              markers: [
                if (pickup != null)
                  marker(
                    pickup,
                    Icons.inventory_2_outlined,
                    c.brandTeal600,
                    'Pickup',
                  ),
                if (dropoff != null)
                  marker(dropoff, Icons.flag_outlined, c.info, 'Destination'),
              ],
            ),
            AnimatedBuilder(
              animation: _positionMotion,
              builder: (context, _) {
                LatLng? position = rider;
                double bearing = widget.location?.heading ?? 0;
                if (widget.demo && pickup != null && dropoff != null) {
                  if (trip?.statusCode == TripStatusCode.arrivedPickup) {
                    position = pickup;
                  } else {
                    final progress =
                        _positionMotion.value * (demoPath.length - 1);
                    final segment = progress.floor().clamp(
                      0,
                      demoPath.length - 2,
                    );
                    final fraction = progress - segment;
                    final a = demoPath[segment], b = demoPath[segment + 1];
                    position = LatLng(
                      a.latitude + (b.latitude - a.latitude) * fraction,
                      a.longitude + (b.longitude - a.longitude) * fraction,
                    );
                    bearing =
                        math.atan2(
                          b.longitude - a.longitude,
                          b.latitude - a.latitude,
                        ) *
                        180 /
                        math.pi;
                    if (_positionMotion.status == AnimationStatus.reverse)
                      bearing += 180;
                  }
                } else if (_previousPosition != null && rider != null) {
                  final t = Curves.easeOutCubic.transform(
                    _positionMotion.value,
                  );
                  position = LatLng(
                    _previousPosition!.latitude +
                        (rider.latitude - _previousPosition!.latitude) * t,
                    _previousPosition!.longitude +
                        (rider.longitude - _previousPosition!.longitude) * t,
                  );
                }
                return MarkerLayer(
                  markers: [
                    if (position != null)
                      Marker(
                        point: position,
                        width: 90,
                        height: 90,
                        rotate: true,
                        child: Transform.rotate(
                          angle: bearing * math.pi / 180,
                          child: DriverPin(online: widget.online),
                        ),
                      ),
                  ],
                );
              },
            ),
            const RichAttributionWidget(
              attributions: [
                TextSourceAttribution('OpenStreetMap contributors'),
              ],
            ),
          ],
        ),
        Positioned(
          right: 16,
          bottom: 260,
          child: Material(
            color: c.surface,
            elevation: 3,
            shadowColor: Colors.black26,
            borderRadius: BorderRadius.circular(18),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                if (trip != null)
                  IconButton(
                    tooltip: 'Show pickup and destination',
                    onPressed: _fitTrip,
                    icon: const Icon(Icons.route_outlined),
                  ),
                IconButton(
                  tooltip: 'Reset map bearing',
                  onPressed: () {
                    if (_ready) _map.rotate(0);
                  },
                  icon: const Icon(Icons.explore_outlined),
                ),
                IconButton(
                  tooltip: 'Centre on your location',
                  onPressed: rider == null ? null : () => _recenter(rider),
                  icon: const Icon(Icons.my_location_rounded),
                ),
              ],
            ),
          ),
        ),
        if (trip != null && !widget.demo && route.isEmpty)
          Positioned(
            left: 16,
            right: 80,
            top: 190,
            child: Material(
              color: c.surface,
              borderRadius: BorderRadius.circular(14),
              child: Padding(
                padding: const EdgeInsets.all(10),
                child: Text(
                  'Road route unavailable. Pickup and destination are shown.',
                  style: Theme.of(context).textTheme.bodySmall,
                ),
              ),
            ),
          ),
        if (_tileError)
          Positioned(
            left: 16,
            right: 80,
            bottom: 270,
            child: Material(
              color: c.surface,
              borderRadius: BorderRadius.circular(16),
              child: Padding(
                padding: const EdgeInsets.all(12),
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      'Map could not load.',
                      style: Theme.of(context).textTheme.bodySmall,
                    ),
                    TextButton(
                      onPressed: () => setState(() {
                        _tileError = false;
                        _tileGeneration++;
                      }),
                      child: const Text('Retry map'),
                    ),
                  ],
                ),
              ),
            ),
          ),
        Positioned(
          left: 20,
          right: 90,
          bottom: 235,
          child: IgnorePointer(
            child: Text(
              widget.demo
                  ? 'Demo route / \u00a9 OpenStreetMap contributors'
                  : '\u00a9 OpenStreetMap contributors',
              style: Theme.of(context).textTheme.labelSmall?.copyWith(
                color: c.textMuted,
                fontSize: 9,
                letterSpacing: 1.3,
              ),
            ),
          ),
        ),
        if (widget.stale)
          Positioned(
            left: 16,
            right: 76,
            top: 190,
            child: Material(
              color: c.warningSoft,
              borderRadius: BorderRadius.circular(14),
              child: Padding(
                padding: const EdgeInsets.all(12),
                child: Text(
                  'Location is out of date',
                  style: TextStyle(color: c.text),
                ),
              ),
            ),
          ),
      ],
    );
  }
}
