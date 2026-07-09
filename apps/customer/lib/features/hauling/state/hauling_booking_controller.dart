import 'dart:async';
import 'dart:math' as math;

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/foundation.dart';

import '../../auth/models/customer_auth_models.dart';
import '../../auth/state/customer_auth_controller.dart';
import '../../wallet/data/wallet_api.dart';
import '../data/customer_realtime_listener.dart';
import '../data/hauling_api.dart';
import '../models/hauling_models.dart';

// ─── Flow status ──────────────────────────────────────────────────────────────

enum HaulingFlowStatus {
  idle,
  details,
  packageInfo,
  locationEntry,
  checkingAvailability,
  unavailable,
  payment,
  paymentProcessing,
  searching,
  activeTrip,
  review,
  completed,
  cancelled,
  error,
}

// ─── State ────────────────────────────────────────────────────────────────────

class HaulingBookingState {
  const HaulingBookingState({
    required this.status,
    this.isLoading = false,
    this.error,
    this.availability,
    // trip progress (0..1 along pickup→destination during cargo leg)
    this.tripProgress = 0.0,
    this.tripStartedAt,
    // location inputs
    this.pickupAddress = '',
    this.pickupLat = 0.0,
    this.pickupLng = 0.0,
    this.dropoffAddress = '',
    this.dropoffLat = 0.0,
    this.dropoffLng = 0.0,
    // form selections
    this.weightCategory,
    this.truckTypeOption,
    this.cargoDescription = '',
    this.requiresHelpers = false,
    this.helperCount = 0,
    this.scheduledAt,
    // derived weight (set when weightCategory is chosen)
    this.cargoWeightKg = 100,
    // package info
    this.receiverName = '',
    this.receiverPhone = '',
    this.packageContent = '',
    this.packageSize = '',
    this.isFragile = false,
    // review
    this.reviewRating = 0,
    this.reviewText = '',
    this.recommendsDriver,
    // payment
    this.paymentMethod,
    this.walletBalanceKobo,
    // fare + booking
    this.fareEstimate,
    this.activeBooking,
    this.bookingHistory = const [],
    // provider snapshot (loaded when booking is accepted)
    this.providerSnapshot,
    this.truckSnapshot,
    // live provider location (driver map marker during the trip)
    this.providerLocation,
  });

  const HaulingBookingState.idle() : this(status: HaulingFlowStatus.idle);

  final HaulingFlowStatus status;
  final bool isLoading;
  final String? error;
  final AvailabilityResult? availability;

  final double tripProgress;
  final DateTime? tripStartedAt;

  final String pickupAddress;
  final double pickupLat;
  final double pickupLng;
  final String dropoffAddress;
  final double dropoffLat;
  final double dropoffLng;

  final WeightCategory? weightCategory;
  final HaulingTruckTypeOption? truckTypeOption;
  final String cargoDescription;
  final bool requiresHelpers;
  final int helperCount;
  final DateTime? scheduledAt;
  final int cargoWeightKg;

  final String receiverName;
  final String receiverPhone;
  final String packageContent;
  final String packageSize;
  final bool isFragile;

  final int reviewRating;
  final String reviewText;
  final bool? recommendsDriver;

  final String? paymentMethod;
  final int? walletBalanceKobo;

  final FareEstimate? fareEstimate;
  final HaulageBooking? activeBooking;
  final List<HaulageBooking> bookingHistory;

  final ProviderSnapshot? providerSnapshot;
  final TruckSnapshot? truckSnapshot;
  final ProviderLocation? providerLocation;

  double get walletBalanceNaira => (walletBalanceKobo ?? 0) / 100;
  bool get locationsReady => pickupAddress.isNotEmpty && dropoffAddress.isNotEmpty;
  bool get detailsReady =>
      cargoDescription.isNotEmpty &&
      weightCategory != null &&
      truckTypeOption != null &&
      (requiresHelpers ? helperCount > 0 : true);

  HaulingBookingState copyWith({
    HaulingFlowStatus? status,
    bool? isLoading,
    String? error,
    AvailabilityResult? availability,
    double? tripProgress,
    DateTime? tripStartedAt,
    bool clearTripProgress = false,
    String? pickupAddress,
    double? pickupLat,
    double? pickupLng,
    String? dropoffAddress,
    double? dropoffLat,
    double? dropoffLng,
    WeightCategory? weightCategory,
    HaulingTruckTypeOption? truckTypeOption,
    String? cargoDescription,
    bool? requiresHelpers,
    int? helperCount,
    DateTime? scheduledAt,
    int? cargoWeightKg,
    String? receiverName,
    String? receiverPhone,
    String? packageContent,
    String? packageSize,
    bool? isFragile,
    int? reviewRating,
    String? reviewText,
    bool? recommendsDriver,
    String? paymentMethod,
    int? walletBalanceKobo,
    FareEstimate? fareEstimate,
    HaulageBooking? activeBooking,
    List<HaulageBooking>? bookingHistory,
    ProviderSnapshot? providerSnapshot,
    TruckSnapshot? truckSnapshot,
    ProviderLocation? providerLocation,
    bool clearError = false,
    bool clearFare = false,
    bool clearBooking = false,
    bool clearScheduledAt = false,
    bool clearProviderSnapshot = false,
    bool clearRecommendsDriver = false,
  }) {
    return HaulingBookingState(
      status: status ?? this.status,
      isLoading: isLoading ?? this.isLoading,
      error: clearError ? null : (error ?? this.error),
      availability: availability ?? this.availability,
      tripProgress: clearTripProgress ? 0.0 : (tripProgress ?? this.tripProgress),
      tripStartedAt: clearTripProgress ? null : (tripStartedAt ?? this.tripStartedAt),
      pickupAddress: pickupAddress ?? this.pickupAddress,
      pickupLat: pickupLat ?? this.pickupLat,
      pickupLng: pickupLng ?? this.pickupLng,
      dropoffAddress: dropoffAddress ?? this.dropoffAddress,
      dropoffLat: dropoffLat ?? this.dropoffLat,
      dropoffLng: dropoffLng ?? this.dropoffLng,
      weightCategory: weightCategory ?? this.weightCategory,
      truckTypeOption: truckTypeOption ?? this.truckTypeOption,
      cargoDescription: cargoDescription ?? this.cargoDescription,
      requiresHelpers: requiresHelpers ?? this.requiresHelpers,
      helperCount: helperCount ?? this.helperCount,
      scheduledAt: clearScheduledAt ? null : (scheduledAt ?? this.scheduledAt),
      cargoWeightKg: cargoWeightKg ?? this.cargoWeightKg,
      receiverName: receiverName ?? this.receiverName,
      receiverPhone: receiverPhone ?? this.receiverPhone,
      packageContent: packageContent ?? this.packageContent,
      packageSize: packageSize ?? this.packageSize,
      isFragile: isFragile ?? this.isFragile,
      reviewRating: reviewRating ?? this.reviewRating,
      reviewText: reviewText ?? this.reviewText,
      recommendsDriver: clearRecommendsDriver ? null : (recommendsDriver ?? this.recommendsDriver),
      paymentMethod: paymentMethod ?? this.paymentMethod,
      walletBalanceKobo: walletBalanceKobo ?? this.walletBalanceKobo,
      fareEstimate: clearFare ? null : (fareEstimate ?? this.fareEstimate),
      activeBooking: clearBooking ? null : (activeBooking ?? this.activeBooking),
      bookingHistory: bookingHistory ?? this.bookingHistory,
      providerSnapshot: clearProviderSnapshot ? null : (providerSnapshot ?? this.providerSnapshot),
      truckSnapshot: clearProviderSnapshot ? null : (truckSnapshot ?? this.truckSnapshot),
      providerLocation: clearProviderSnapshot ? null : (providerLocation ?? this.providerLocation),
    );
  }
}

// ─── Controller ───────────────────────────────────────────────────────────────

class HaulingBookingController extends ChangeNotifier {
  HaulingBookingController({
    required HaulingApi api,
    required CustomerAuthController authController,
    required WalletApi walletApi,
    CustomerRealtimeListener Function(
      String? Function() accessToken,
      void Function(String eventType) onEvent,
    )? realtimeListenerFactory,
    this.savedLocationsLoader,
  })  : _api = api,
        _auth = authController,
        _walletApi = walletApi,
        _realtimeFactory = realtimeListenerFactory;

  final HaulingApi _api;
  final CustomerAuthController _auth;
  final WalletApi _walletApi;

  /// Fetches the customer's saved locations so the location entry view can
  /// offer them as one-tap fills. Optional — the view hides the section when
  /// unset (tests/headless configs) or when the fetch fails.
  final Future<List<SavedLocation>> Function()? savedLocationsLoader;

  /// Exposed so views (e.g. the Trips list) can lazily fetch provider snapshots
  /// for display without duplicating the HTTP client.
  HaulingApi get api => _api;

  /// Builds the realtime websocket listener (fast path for booking updates).
  /// Optional so tests/headless configs can omit it; polling is the fallback.
  final CustomerRealtimeListener Function(
    String? Function(),
    void Function(String),
  )? _realtimeFactory;
  CustomerRealtimeListener? _realtime;

  Timer? _pollTimer;
  Timer? _searchTimeoutTimer;
  Timer? _locationTimer;

  HaulingBookingState _state = const HaulingBookingState.idle();
  HaulingBookingState get state => _state;

  /// Guards the booking/location polls: one request in flight at a time, and a
  /// response fetched before a local flow mutation (cancel/reset/review/open)
  /// is dropped — otherwise a poll that started just before a cancel can land
  /// after it and revive the searching/active screen with stale data.
  bool _pollInFlight = false;
  int _stateGeneration = 0;

  void _bumpGeneration() => _stateGeneration++;

  void _emit(HaulingBookingState next) {
    _state = next;
    notifyListeners();
  }

  String? _accessToken() => _auth.state.session?.accessToken;

  // ─── Flow entry ────────────────────────────────────────────────────────────

  void startHaulingFlow() {
    _emit(_state.copyWith(
      status: HaulingFlowStatus.locationEntry,
      clearError: true,
    ));
  }

  /// Re-opens an in-progress booking (active or still searching) into the live
  /// flow and resumes polling. `_applyBookingUpdate` maps the booking to the
  /// right flow status (activeTrip / searching / review) and kicks off the
  /// provider/location fetches.
  void openActiveBooking(HaulageBooking booking) {
    _bumpGeneration();
    _emit(_state.copyWith(
      activeBooking: booking,
      clearError: true,
      clearProviderSnapshot: true,
    ));
    _startPolling();
    _applyBookingUpdate(booking);
  }

  /// Prefills the booking form from a previous trip and starts the normal flow
  /// at location entry, so the customer can confirm addresses and walk through
  /// details → package → payment with everything pre-filled.
  void rebookFrom(HaulageBooking booking) {
    reset();
    setPickupLocation(booking.pickupAddress, booking.pickupLat, booking.pickupLng);
    if (booking.dropoffAddress.isNotEmpty) {
      setDropoffLocation(booking.dropoffAddress, booking.dropoffLat, booking.dropoffLng);
    }

    final weight = WeightCategory.fromName(booking.weightCategory);
    if (weight != null) setWeightCategory(weight);

    final truckType = HaulingTruckTypeOption.fromApiValue(booking.preferredTruckType);
    if (truckType != null) setTruckTypeOption(truckType);

    if (booking.cargoDescription.isNotEmpty) setCargoDescription(booking.cargoDescription);
    setRequiresHelpers(booking.requiresHelpers);
    if (booking.requiresHelpers) setHelperCount(booking.helperCount);

    setReceiverName(booking.receiverName);
    setReceiverPhone(booking.receiverPhone);
    setPackageContent(booking.packageContent);
    setPackageSize(booking.packageSize);
    setIsFragile(booking.isFragile);

    startHaulingFlow();
  }

  // ─── Location entry ────────────────────────────────────────────────────────

  void setPickupLocation(String address, double lat, double lng) {
    _emit(_state.copyWith(
      pickupAddress: address,
      pickupLat: lat,
      pickupLng: lng,
      clearFare: true,
    ));
  }

  void setDropoffLocation(String address, double lat, double lng) {
    _emit(_state.copyWith(
      dropoffAddress: address,
      dropoffLat: lat,
      dropoffLng: lng,
      clearFare: true,
    ));
  }

  Future<void> checkAvailabilityAndProceed() async {
    final token = _accessToken();
    if (token == null) return;

    _emit(_state.copyWith(
      status: HaulingFlowStatus.checkingAvailability,
      isLoading: true,
      clearError: true,
    ));

    try {
      final result = await _api.checkAvailability(accessToken: token);
      if (result.available) {
        _emit(_state.copyWith(
          status: HaulingFlowStatus.details,
          isLoading: false,
          availability: result,
        ));
        _fetchPreviewFare();
      } else {
        _emit(_state.copyWith(
          status: HaulingFlowStatus.unavailable,
          isLoading: false,
          availability: result,
        ));
      }
    } catch (e) {
      _emit(_state.copyWith(
        status: HaulingFlowStatus.error,
        isLoading: false,
        error: _errorMessage(e),
      ));
    }
  }

  void _fetchPreviewFare() {
    if (_state.pickupLat == 0 && _state.pickupLng == 0) return;
    // Use the real cargo weight/helpers once the customer has chosen them, so
    // the preview matches what's charged. Falls back to a light default before
    // details are entered.
    final weight = _state.weightCategory != null ? _state.cargoWeightKg : 100;
    _api.estimateFare(
      pickupLat: _state.pickupLat,
      pickupLng: _state.pickupLng,
      dropoffLat: _state.dropoffLat,
      dropoffLng: _state.dropoffLng,
      cargoWeightKg: weight,
      helperCount: _state.helperCount,
    ).then((estimate) {
      _emit(_state.copyWith(fareEstimate: estimate));
    }).catchError((_) {});
  }

  // ─── Step transitions ──────────────────────────────────────────────────────

  void proceedFromDetailsToPackageInfo() {
    _emit(_state.copyWith(status: HaulingFlowStatus.packageInfo, clearError: true));
  }

  void proceedFromPackageInfoToPayment() {
    initiatePayment();
  }

  // ─── Details form ──────────────────────────────────────────────────────────

  void setWeightCategory(WeightCategory cat) {
    _emit(_state.copyWith(
      weightCategory: cat,
      cargoWeightKg: cat.kg,
      clearFare: true,
    ));
  }

  void setTruckTypeOption(HaulingTruckTypeOption opt) {
    _emit(_state.copyWith(truckTypeOption: opt));
  }

  void setCargoDescription(String d) {
    _emit(_state.copyWith(cargoDescription: d));
  }

  void setRequiresHelpers(bool v) {
    _emit(_state.copyWith(
      requiresHelpers: v,
      helperCount: v ? _state.helperCount : 0,
    ));
  }

  void setHelperCount(int n) {
    _emit(_state.copyWith(helperCount: n));
  }

  void setScheduledAt(DateTime? dt) {
    if (dt == null) {
      _emit(_state.copyWith(clearScheduledAt: true));
    } else {
      _emit(_state.copyWith(scheduledAt: dt));
    }
  }

  // ─── Package info setters ──────────────────────────────────────────────────

  void setReceiverName(String v) => _emit(_state.copyWith(receiverName: v));
  void setReceiverPhone(String v) => _emit(_state.copyWith(receiverPhone: v));
  void setPackageContent(String v) => _emit(_state.copyWith(packageContent: v));
  void setPackageSize(String v) => _emit(_state.copyWith(packageSize: v));
  void setIsFragile(bool v) => _emit(_state.copyWith(isFragile: v));

  // ─── Review setters ───────────────────────────────────────────────────────

  void setReviewRating(int v) => _emit(_state.copyWith(reviewRating: v));
  void setReviewText(String v) => _emit(_state.copyWith(reviewText: v));
  void setRecommendsDriver(bool? v) {
    if (v == null) {
      _emit(_state.copyWith(clearRecommendsDriver: true));
    } else {
      _emit(_state.copyWith(recommendsDriver: v));
    }
  }

  Future<void> submitReview() async {
    final token = _accessToken();
    final bookingId = _state.activeBooking?.id;
    if (token == null || bookingId == null) return;

    _emit(_state.copyWith(isLoading: true, clearError: true));
    try {
      await _api.submitReview(
        accessToken: token,
        bookingId: bookingId,
        rating: _state.reviewRating,
        reviewText: _state.reviewText,
        recommendsDriver: _state.recommendsDriver,
      );
      _bumpGeneration();
      _emit(_state.copyWith(
        status: HaulingFlowStatus.completed,
        isLoading: false,
      ));
      // Refresh the history so the just-completed trip shows under "Past".
      unawaited(loadHistory());
    } catch (e) {
      _emit(_state.copyWith(isLoading: false, error: _errorMessage(e)));
    }
  }

  void skipReview() {
    _bumpGeneration();
    _emit(_state.copyWith(status: HaulingFlowStatus.completed));
    unawaited(loadHistory());
  }

  /// Submits a review for an arbitrary completed booking (used from the Trips
  /// detail screen, where the booking isn't the live `activeBooking`). Returns
  /// the created review on success, or throws so the caller can surface the
  /// error. Refreshes history afterwards.
  Future<BookingReview> submitReviewForBooking({
    required String bookingId,
    required int rating,
    String reviewText = '',
    bool? recommendsDriver,
  }) async {
    final token = _accessToken();
    if (token == null) {
      throw StateError('Not authenticated');
    }
    final review = await _api.submitReview(
      accessToken: token,
      bookingId: bookingId,
      rating: rating,
      reviewText: reviewText,
      recommendsDriver: recommendsDriver,
    );
    unawaited(loadHistory());
    return review;
  }

  // ─── Payment initiation ────────────────────────────────────────────────────

  Future<void> initiatePayment() async {
    final token = _accessToken();
    if (token == null) return;

    _emit(_state.copyWith(isLoading: true, clearError: true));

    try {
      final estimate = await _api.estimateFare(
        pickupLat: _state.pickupLat,
        pickupLng: _state.pickupLng,
        dropoffLat: _state.dropoffLat,
        dropoffLng: _state.dropoffLng,
        cargoWeightKg: _state.cargoWeightKg,
        helperCount: _state.helperCount,
      );

      int? walletBalance;
      try {
        final wallet = await _walletApi.getWallet(accessToken: token);
        walletBalance = wallet.availableKobo;
      } catch (_) {
        // non-fatal: wallet fetch failure just means balance won't show
      }

      _emit(_state.copyWith(
        status: HaulingFlowStatus.payment,
        isLoading: false,
        fareEstimate: estimate,
        walletBalanceKobo: walletBalance ?? _state.walletBalanceKobo,
        // Wallet is the only payment method; preselect it.
        paymentMethod: _state.paymentMethod ?? 'wallet',
      ));
    } catch (e) {
      _emit(_state.copyWith(isLoading: false, error: _errorMessage(e)));
    }
  }

  /// Booking is wallet-only. The wallet is charged (held) when a provider
  /// accepts (see the service's ensurePaymentSecured); we just create the
  /// booking and start searching.
  Future<void> confirmPayment() async {
    final token = _accessToken();
    if (token == null) return;

    _emit(_state.copyWith(
      status: HaulingFlowStatus.paymentProcessing,
      isLoading: true,
      clearError: true,
    ));

    await _createBookingAndStartSearch(token);
  }

  /// Exposes what the payment view needs to launch the shared wallet top-up
  /// flow (the funding screen wires its own controller from these).
  WalletApi get walletApi => _walletApi;
  String? get accessTokenForWallet => _accessToken();
  String get customerEmail {
    final s = _auth.state;
    return s.profileEmail.trim().isNotEmpty ? s.profileEmail.trim() : s.email.trim();
  }

  /// Re-fetches the wallet balance (e.g. after returning from a top-up) so the
  /// payment screen can re-enable Confirm once the balance covers the fare.
  Future<void> refreshWalletBalance() async {
    final token = _accessToken();
    if (token == null) return;
    try {
      final wallet = await _walletApi.getWallet(accessToken: token);
      _emit(_state.copyWith(walletBalanceKobo: wallet.availableKobo));
    } catch (_) {
      // non-fatal: balance just won't update
    }
  }

  // ─── Booking creation ──────────────────────────────────────────────────────

  Future<void> _createBookingAndStartSearch(String token) async {
    try {
      final booking = await _api.createBooking(
        accessToken: token,
        pickupAddress: _state.pickupAddress,
        pickupLat: _state.pickupLat,
        pickupLng: _state.pickupLng,
        dropoffAddress: _state.dropoffAddress,
        dropoffLat: _state.dropoffLat,
        dropoffLng: _state.dropoffLng,
        preferredTruckType: _state.truckTypeOption?.apiValue ?? '',
        cargoWeightKg: _state.cargoWeightKg,
        cargoDescription: _state.cargoDescription,
        requiresHelpers: _state.requiresHelpers,
        helperCount: _state.helperCount,
        weightCategory: _state.weightCategory?.name ?? '',
        receiverName: _state.receiverName,
        receiverPhone: _state.receiverPhone,
        packageContent: _state.packageContent,
        packageSize: _state.packageSize,
        isFragile: _state.isFragile,
        paymentMethod: 'wallet',
        scheduledAt: _state.scheduledAt,
      );
      _emit(_state.copyWith(
        status: HaulingFlowStatus.searching,
        isLoading: false,
        activeBooking: booking,
      ));
      _startPolling();
    } catch (e) {
      _emit(_state.copyWith(
        status: HaulingFlowStatus.payment,
        isLoading: false,
        error: _errorMessage(e),
      ));
    }
  }

  // ─── Cancel ────────────────────────────────────────────────────────────────

  Future<void> cancelBooking({String reason = ''}) async {
    final token = _accessToken();
    final bookingId = _state.activeBooking?.id;
    if (token == null || bookingId == null) return;

    _emit(_state.copyWith(isLoading: true, clearError: true));
    try {
      final updated = await _api.cancelBooking(
        accessToken: token,
        bookingId: bookingId,
        reason: reason,
      );
      _bumpGeneration();
      _stopPolling();
      _emit(_state.copyWith(
        status: HaulingFlowStatus.cancelled,
        isLoading: false,
        activeBooking: updated,
      ));
    } catch (e) {
      _emit(_state.copyWith(isLoading: false, error: _errorMessage(e)));
    }
  }

  /// "Edit Destination" while still connecting to a driver (Figma 1736):
  /// the booking is already created server-side, so editing means cancelling
  /// the current search and returning to location entry with the addresses
  /// still filled in.
  Future<void> editDestinationDuringSearch() async {
    await cancelBooking(reason: 'edit_destination');
    if (_state.status == HaulingFlowStatus.cancelled) {
      startHaulingFlow();
    }
  }

  // ─── History ───────────────────────────────────────────────────────────────

  Future<void> loadHistory() async {
    final token = _accessToken();
    if (token == null) return;
    try {
      final bookings = await _api.listBookings(accessToken: token);
      _emit(_state.copyWith(bookingHistory: bookings));
    } catch (_) {
      // silently fail — history is non-critical
    }
  }

  // ─── Polling ───────────────────────────────────────────────────────────────

  void _startPolling() {
    _pollTimer?.cancel();
    _pollTimer = Timer.periodic(const Duration(seconds: 5), (_) => _pollBookingStatus());
    _startSearchTimeout();
    _startRealtime();
  }

  void _stopPolling() {
    _pollTimer?.cancel();
    _pollTimer = null;
    _stopSearchTimeout();
    _stopLocationPolling();
    _stopRealtime();
  }

  // ─── Realtime (websocket fast path) ─────────────────────────────────────────

  void _startRealtime() {
    if (_realtimeFactory == null) return;
    _realtime ??= _realtimeFactory(() => _accessToken(), _onRealtimeEvent);
    _realtime!.start();
  }

  void _stopRealtime() {
    _realtime?.stop();
  }

  /// A pushed event means booking state likely changed — refresh now instead of
  /// waiting for the next 5s poll.
  void _onRealtimeEvent(String _) {
    _pollBookingStatus();
  }

  /// External fast-path poke (e.g. a foreground FCM push): refresh the active
  /// booking immediately when one is being tracked. No-op otherwise.
  void pokeRefresh() {
    if (_pollTimer == null) return;
    _pollBookingStatus();
  }

  // ─── Live driver location ────────────────────────────────────────────────────

  void _startLocationPolling() {
    if (_locationTimer != null) return;
    _pollDriverLocation();
    _locationTimer = Timer.periodic(const Duration(seconds: 8), (_) => _pollDriverLocation());
  }

  void _stopLocationPolling() {
    _locationTimer?.cancel();
    _locationTimer = null;
  }

  Future<void> _pollDriverLocation() async {
    final token = _accessToken();
    final bookingId = _state.activeBooking?.id;
    if (token == null || bookingId == null) return;
    final generation = _stateGeneration;
    try {
      final loc = await _api.getBookingLocation(accessToken: token, bookingId: bookingId);
      // Flow moved on (cancelled/reset) while this was in flight — drop it.
      if (generation != _stateGeneration) return;
      final booking = _state.activeBooking;
      if (booking == null) return;
      if (loc.hasFix) {
        final progress = _computeTripProgress(booking, location: loc);
        _emit(_state.copyWith(providerLocation: loc, tripProgress: progress));
      } else {
        // No fix: still advance the time-based bar.
        _emit(_state.copyWith(tripProgress: _computeTripProgress(booking)));
      }
    } catch (_) {
      // non-fatal — advance time-based bar anyway
      if (generation != _stateGeneration) return;
      final booking = _state.activeBooking;
      if (booking != null) {
        _emit(_state.copyWith(tripProgress: _computeTripProgress(booking)));
      }
    }
  }

  /// How long to wait for a driver before giving up. On expiry the booking is
  /// cancelled server-side and the cancelled view explains that no trucks were
  /// found.
  static const searchTimeout = Duration(seconds: 90);

  void _startSearchTimeout() {
    _searchTimeoutTimer?.cancel();
    _searchTimeoutTimer = Timer(searchTimeout, () async {
      if (_state.status != HaulingFlowStatus.searching) return;
      // Cancel server-side too — otherwise the booking stays live and a provider
      // could still accept (and, for wallet, be charged) after the customer has
      // given up. cancelBooking stops polling and surfaces any error itself.
      final bookingId = _state.activeBooking?.id;
      final token = _accessToken();
      if (bookingId != null && token != null) {
        try {
          final updated = await _api.cancelBooking(
            accessToken: token,
            bookingId: bookingId,
            reason: 'search_timeout',
          );
          _bumpGeneration();
          _stopPolling();
          _emit(_state.copyWith(
            status: HaulingFlowStatus.cancelled,
            isLoading: false,
            activeBooking: updated,
            error: 'No trucks found nearby. Please try again later.',
          ));
          return;
        } catch (_) {
          // Fall through to local cancel if the server call fails.
        }
      }
      _bumpGeneration();
      _stopPolling();
      _emit(_state.copyWith(
        status: HaulingFlowStatus.cancelled,
        isLoading: false,
        error: 'No trucks found nearby. Please try again later.',
      ));
    });
  }

  void _stopSearchTimeout() {
    _searchTimeoutTimer?.cancel();
    _searchTimeoutTimer = null;
  }

  // ─── Trip progress computation ──────────────────────────────────────────────

  /// Returns a 0..1 value for the cargo-leg progress slider:
  /// - GPS branch: `1 - driverRemaining / totalPickupToDropoff` (real geography)
  /// - Time branch: elapsed / ETA (fallback when no GPS fix)
  /// Both branches are clamped to [0.05, 0.95] and monotonic (never regress).
  double _computeTripProgress(HaulageBooking booking, {ProviderLocation? location}) {
    final prev = _state.tripProgress;

    final cargoStatuses = [
      HaulingBookingStatus.pickedUp,
      HaulingBookingStatus.enRouteDelivery,
    ];
    if (!cargoStatuses.contains(booking.status)) return prev;

    final totalKm = _haversine(
      booking.pickupLat, booking.pickupLng,
      booking.dropoffLat, booking.dropoffLng,
    );
    if (totalKm < 0.05) return prev;

    double computed;
    final loc = location ?? _state.providerLocation;
    if (loc != null && loc.hasFix) {
      final remaining = _haversine(loc.lat, loc.lng, booking.dropoffLat, booking.dropoffLng);
      computed = (1 - remaining / totalKm).clamp(0.05, 0.95);
    } else {
      // Time-based fallback: estimate progress from elapsed time since pickup.
      final started = _state.tripStartedAt;
      if (started == null) return prev;
      final etaMinutes = (totalKm / 30.0 * 60).ceil().clamp(1, 300);
      final elapsedSeconds = DateTime.now().difference(started).inSeconds;
      computed = (elapsedSeconds / (etaMinutes * 60)).clamp(0.05, 0.95);
    }

    // Monotonic: never regress even if GPS jitters or the clock re-anchors.
    return math.max(prev, computed);
  }

  static double _haversine(double lat1, double lng1, double lat2, double lng2) {
    const r = 6371.0;
    final dLat = (lat2 - lat1) * math.pi / 180;
    final dLng = (lng2 - lng1) * math.pi / 180;
    final a = math.sin(dLat / 2) * math.sin(dLat / 2) +
        math.cos(lat1 * math.pi / 180) *
            math.cos(lat2 * math.pi / 180) *
            math.sin(dLng / 2) *
            math.sin(dLng / 2);
    return 2 * r * math.asin(math.min(1, math.sqrt(a)));
  }

  Future<void> _pollBookingStatus() async {
    final token = _accessToken();
    final bookingId = _state.activeBooking?.id;
    if (token == null || bookingId == null || _pollInFlight) return;

    _pollInFlight = true;
    final generation = _stateGeneration;
    try {
      final booking = await _api.getBooking(accessToken: token, bookingId: bookingId);
      // A local mutation (cancel/reset/review) happened while this response was
      // in flight — the data is stale, don't let it revive the old screen.
      if (generation != _stateGeneration) return;
      await _applyBookingUpdate(booking);
    } catch (_) {
      // network hiccup — keep polling
    } finally {
      _pollInFlight = false;
    }
  }

  Future<void> _applyBookingUpdate(HaulageBooking booking) async {
    final status = booking.status;

    if (status.isTerminal) {
      _stopPolling();
      final flowStatus = status == HaulingBookingStatus.completed
          ? HaulingFlowStatus.completed
          : HaulingFlowStatus.cancelled;
      _emit(_state.copyWith(status: flowStatus, activeBooking: booking));
      // Keep the Trips list in sync with the terminal outcome.
      unawaited(loadHistory());
      return;
    }

    if (status == HaulingBookingStatus.delivered) {
      _stopPolling();
      // The customer may already have reviewed this trip (e.g. from the Trips
      // detail screen). Skip straight to completed instead of offering a form
      // whose submit the server would reject as a duplicate.
      final token = _accessToken();
      if (token != null) {
        try {
          final existing = await _api.getReviewForBooking(
            accessToken: token,
            bookingId: booking.id,
          );
          if (existing != null) {
            _emit(_state.copyWith(status: HaulingFlowStatus.completed, activeBooking: booking));
            unawaited(loadHistory());
            return;
          }
        } catch (_) {
          // Review lookup failing must not block the flow — show the form; the
          // server still guards against duplicates.
        }
      }
      _emit(_state.copyWith(status: HaulingFlowStatus.review, activeBooking: booking));
      return;
    }

    if (status.isActive) {
      // Trip is live — start streaming the driver's location for the map.
      _startLocationPolling();

      // Anchor the start time the first time we enter the cargo leg so the
      // time-based progress branch has a reference point.
      final cargoStatuses = [
        HaulingBookingStatus.pickedUp,
        HaulingBookingStatus.enRouteDelivery,
      ];
      final startedAt = (cargoStatuses.contains(status) && _state.tripStartedAt == null)
          ? DateTime.now()
          : _state.tripStartedAt;

      _emit(_state.copyWith(
        status: HaulingFlowStatus.activeTrip,
        activeBooking: booking,
        tripStartedAt: startedAt,
        tripProgress: _computeTripProgress(booking),
      ));
      // Load the assigned provider/truck snapshot if not yet loaded.
      _maybeFetchProviderAndTruckInfo(booking);
      return;
    }

    // searching: pendingMatch or awaitingAcceptance
    // Load provider snapshot when we know the provider (awaitingAcceptance).
    // When the booking dropped back to pendingMatch (provider timed out or
    // rejected), clear the previous driver so the searching view returns to the
    // "connecting" placeholder instead of showing a stale driver.
    _emit(_state.copyWith(
      activeBooking: booking,
      clearProviderSnapshot: booking.providerId == null,
    ));
    _maybeFetchProviderAndTruckInfo(booking);
  }

  /// Loads the assigned provider (and truck, when present) for the booking once
  /// a provider is known. The truck is optional: a provider that used "Go
  /// Online" is matched without a specific truck, so `truckId` is null — the
  /// driver card renders fine without it, but the provider details must still
  /// show (otherwise the customer is stuck on the shimmer placeholder).
  void _maybeFetchProviderAndTruckInfo(HaulageBooking booking) {
    final providerId = booking.providerId;
    if (providerId == null) return;
    final token = _accessToken();
    if (token == null) return;

    if (_state.providerSnapshot?.id != providerId) {
      _api.getProvider(accessToken: token, providerId: providerId).then((p) {
        _emit(_state.copyWith(providerSnapshot: p));
      }).catchError((_) {});
    }

    final truckId = booking.truckId;
    if (truckId != null && _state.truckSnapshot?.id != truckId) {
      _api.getTruck(accessToken: token, truckId: truckId).then((t) {
        _emit(_state.copyWith(truckSnapshot: t));
      }).catchError((_) {});
    }
  }

  // ─── Navigation helpers ────────────────────────────────────────────────────

  void reset() {
    _bumpGeneration();
    _stopPolling();
    _emit(const HaulingBookingState.idle());
    // tripProgress/tripStartedAt are reset by the idle constructor defaults.
  }

  void backToDetails() {
    _emit(_state.copyWith(status: HaulingFlowStatus.details, clearFare: true));
  }

  void backToPackageInfo() {
    _emit(_state.copyWith(status: HaulingFlowStatus.packageInfo, clearError: true));
  }

  void backToLocationEntry() {
    _emit(_state.copyWith(status: HaulingFlowStatus.locationEntry, clearError: true));
  }

  // ─── Utilities ─────────────────────────────────────────────────────────────

  String _errorMessage(Object e) {
    if (e is ApiException) return e.message;
    return e.toString();
  }

  @override
  void dispose() {
    _stopPolling();
    _stopSearchTimeout();
    _stopLocationPolling();
    _stopRealtime();
    super.dispose();
  }
}
