import 'dart:async';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/foundation.dart';

import '../../auth/models/customer_auth_models.dart';
import '../../auth/state/customer_auth_controller.dart';
import '../../wallet/data/wallet_api.dart';
import '../data/taxi_api.dart';
import '../models/taxi_models.dart';

// ─── Flow status ──────────────────────────────────────────────────────────────

enum TaxiFlowStatus {
  idle,
  modeSelection,
  locationEntry,
  checkingAvailability,
  unavailable,
  fareSelection,
  parcelDetails,
  rideOptions,
  searching,
  activeTrip,
  review,
  completed,
  cancelled,
  error,
}

// ─── State ────────────────────────────────────────────────────────────────────

@immutable
class TaxiBookingState {
  const TaxiBookingState({
    required this.status,
    this.isLoading = false,
    this.error,
    this.availability,
    // location inputs
    this.pickupAddress = '',
    this.pickupLat = 0.0,
    this.pickupLng = 0.0,
    this.dropoffAddress = '',
    this.dropoffLat = 0.0,
    this.dropoffLng = 0.0,
    // ride options
    this.fareTiles = const [],
    this.selectedClass,
    this.rideNotes = '',
    this.walletBalanceKobo,
    this.rideMode = TaxiRideMode.ride,
    this.receiverName = '',
    this.receiverPhone = '',
    this.packageDesc = '',
    this.packageType = '',
    this.packageSize = '',
    this.isFragile = false,
    this.deliveryCredential,
    this.isResendingDeliveryCode = false,
    // booking
    this.activeBooking,
    this.bookingHistory = const [],
    // pickup handshake — the passenger's own code, shown to the driver
    this.pickupCredential,
    this.isResendingCode = false,
    // review
    this.reviewRating = 0,
    this.reviewText = '',
    this.recommendsDriver,
    this.reviews,
  });

  const TaxiBookingState.idle() : this(status: TaxiFlowStatus.idle);

  final TaxiFlowStatus status;
  final bool isLoading;
  final String? error;
  final TaxiAvailability? availability;

  final String pickupAddress;
  final double pickupLat;
  final double pickupLng;
  final String dropoffAddress;
  final double dropoffLat;
  final double dropoffLng;

  final List<TaxiFareTile> fareTiles;
  final TaxiFareTile? selectedClass;
  final String rideNotes;

  /// Wallet available balance, loaded alongside the fare tiles so the confirm
  /// screen can refuse a ride the customer cannot pay for BEFORE booking --
  /// otherwise the failure surfaces at driver-accept time and reads like a
  /// matcher fault. Null means "not loaded yet", which never blocks.
  final int? walletBalanceKobo;

  /// Whether this booking carries the customer or a package. Chosen on the
  /// first screen of the flow and fixed for the rest of it.
  final TaxiRideMode rideMode;
  final String receiverName;
  final String receiverPhone;
  final String packageDesc;
  final String packageType;
  final String packageSize;
  final bool isFragile;

  /// The receiver's drop-off code, fetched once the package is in transit.
  final TaxiHandoverCredential? deliveryCredential;
  final bool isResendingDeliveryCode;

  final TaxiBooking? activeBooking;
  final List<TaxiBooking> bookingHistory;

  final TaxiPickupCredential? pickupCredential;
  final bool isResendingCode;

  final int reviewRating;
  final String reviewText;
  final bool? recommendsDriver;
  final TaxiBookingReviews? reviews;

  bool get hasPickupFix => pickupLat != 0.0 || pickupLng != 0.0;
  bool get hasDropoffFix => dropoffLat != 0.0 || dropoffLng != 0.0;
  /// A parcel needs somewhere to send it and someone to receive it before the
  /// service will accept the booking; the app blocks earlier so the customer
  /// never sees a server-side field error.
  bool get canSubmitParcelDetails =>
      packageDesc.trim().isNotEmpty &&
      receiverName.trim().isNotEmpty &&
      receiverPhone.trim().length >= 10;

  bool get canRequestFare =>
      pickupAddress.trim().isNotEmpty &&
      dropoffAddress.trim().isNotEmpty &&
      hasPickupFix &&
      hasDropoffFix;

  TaxiBookingState copyWith({
    TaxiFlowStatus? status,
    bool? isLoading,
    String? error,
    bool clearError = false,
    TaxiAvailability? availability,
    String? pickupAddress,
    double? pickupLat,
    double? pickupLng,
    String? dropoffAddress,
    double? dropoffLat,
    double? dropoffLng,
    List<TaxiFareTile>? fareTiles,
    TaxiFareTile? selectedClass,
    String? rideNotes,
    int? walletBalanceKobo,
    TaxiRideMode? rideMode,
    String? receiverName,
    String? receiverPhone,
    String? packageDesc,
    String? packageType,
    String? packageSize,
    bool? isFragile,
    TaxiHandoverCredential? deliveryCredential,
    bool? isResendingDeliveryCode,
    TaxiBooking? activeBooking,
    bool clearActiveBooking = false,
    List<TaxiBooking>? bookingHistory,
    TaxiPickupCredential? pickupCredential,
    bool clearPickupCredential = false,
    bool? isResendingCode,
    int? reviewRating,
    String? reviewText,
    bool? recommendsDriver,
    TaxiBookingReviews? reviews,
  }) {
    return TaxiBookingState(
      status: status ?? this.status,
      isLoading: isLoading ?? this.isLoading,
      error: clearError ? null : (error ?? this.error),
      availability: availability ?? this.availability,
      pickupAddress: pickupAddress ?? this.pickupAddress,
      pickupLat: pickupLat ?? this.pickupLat,
      pickupLng: pickupLng ?? this.pickupLng,
      dropoffAddress: dropoffAddress ?? this.dropoffAddress,
      dropoffLat: dropoffLat ?? this.dropoffLat,
      dropoffLng: dropoffLng ?? this.dropoffLng,
      fareTiles: fareTiles ?? this.fareTiles,
      selectedClass: selectedClass ?? this.selectedClass,
      rideNotes: rideNotes ?? this.rideNotes,
      walletBalanceKobo: walletBalanceKobo ?? this.walletBalanceKobo,
      rideMode: rideMode ?? this.rideMode,
      receiverName: receiverName ?? this.receiverName,
      receiverPhone: receiverPhone ?? this.receiverPhone,
      packageDesc: packageDesc ?? this.packageDesc,
      packageType: packageType ?? this.packageType,
      packageSize: packageSize ?? this.packageSize,
      isFragile: isFragile ?? this.isFragile,
      deliveryCredential: deliveryCredential ?? this.deliveryCredential,
      isResendingDeliveryCode:
          isResendingDeliveryCode ?? this.isResendingDeliveryCode,
      activeBooking:
          clearActiveBooking ? null : (activeBooking ?? this.activeBooking),
      bookingHistory: bookingHistory ?? this.bookingHistory,
      pickupCredential: clearPickupCredential
          ? null
          : (pickupCredential ?? this.pickupCredential),
      isResendingCode: isResendingCode ?? this.isResendingCode,
      reviewRating: reviewRating ?? this.reviewRating,
      reviewText: reviewText ?? this.reviewText,
      recommendsDriver: recommendsDriver ?? this.recommendsDriver,
      reviews: reviews ?? this.reviews,
    );
  }
}

// ─── Controller ───────────────────────────────────────────────────────────────

/// Drives the passenger ride flow against driver-taxi-service.
///
/// Deliberately simpler than the dispatch controller in two ways, both forced
/// by the backend rather than chosen: taxi exposes no customer-facing driver
/// location endpoint (so there is no live car marker to poll) and no customer
/// realtime channel (so polling is the only update path, not a fallback).
///
/// Rides are wallet-only, like hauling and dispatch. The wallet is charged on
/// driver acceptance, so this controller loads the balance up front purely to
/// let the confirm screen block an unpayable ride before it is created.
class TaxiBookingController extends ChangeNotifier {
  TaxiBookingController({
    required TaxiApi api,
    required CustomerAuthController authController,
    required WalletApi walletApi,
    this.savedLocationsLoader,
  }) : _api = api,
       _auth = authController,
       _walletApi = walletApi;

  final TaxiApi _api;
  final CustomerAuthController _auth;
  final WalletApi _walletApi;

  /// Fetches the customer's saved locations so the location entry view can
  /// offer them as one-tap fills. Optional — the view hides the section when
  /// unset or when the fetch fails.
  final Future<List<SavedLocation>> Function()? savedLocationsLoader;

  /// Exposed so views (e.g. the Trips list) can read history without
  /// duplicating the HTTP client.
  TaxiApi get api => _api;

  Timer? _pollTimer;
  Timer? _searchTimeoutTimer;

  TaxiBookingState _state = const TaxiBookingState.idle();
  TaxiBookingState get state => _state;

  /// Guards the booking poll: one request in flight at a time, and any
  /// response fetched before a local flow mutation (cancel/reset/review) is
  /// dropped — otherwise a poll started just before a cancel lands after it
  /// and revives the searching screen with stale data.
  bool _pollInFlight = false;
  int _stateGeneration = 0;

  void _bumpGeneration() => _stateGeneration++;

  void _emit(TaxiBookingState next) {
    _state = next;
    notifyListeners();
  }

  String? _accessToken() => _auth.state.session?.accessToken;

  void _requireAuth() => _auth.handleAuthFailure();

  // ─── Flow entry ────────────────────────────────────────────────────────────

  void startTaxiFlow() {
    _emit(_state.copyWith(
      status: TaxiFlowStatus.modeSelection,
      clearError: true,
    ));
  }

  /// Locks in ride-versus-parcel and moves on to where/what.
  void selectMode(TaxiRideMode mode) {
    _emit(_state.copyWith(
      status: TaxiFlowStatus.locationEntry,
      rideMode: mode,
      clearError: true,
    ));
  }

  void backToModeSelection() {
    _emit(_state.copyWith(
      status: TaxiFlowStatus.modeSelection,
      clearError: true,
    ));
  }

  void reset() {
    _bumpGeneration();
    _stopPolling();
    _stopSearchTimeout();
    _emit(const TaxiBookingState.idle());
  }

  /// Re-opens an in-progress ride (active or still searching) and resumes
  /// polling — used when returning from the Trips list.
  void openActiveBooking(TaxiBooking booking) {
    _bumpGeneration();
    _emit(_state.copyWith(
      status: booking.status.isSearching
          ? TaxiFlowStatus.searching
          : TaxiFlowStatus.activeTrip,
      activeBooking: booking,
      clearError: true,
    ));
    _startPolling();
    if (booking.status.isSearching) _startSearchTimeout();
  }

  // ─── Location entry ────────────────────────────────────────────────────────

  void setPickup({required String address, double? lat, double? lng}) {
    _emit(_state.copyWith(
      pickupAddress: address,
      pickupLat: lat,
      pickupLng: lng,
      clearError: true,
    ));
  }

  void setDropoff({required String address, double? lat, double? lng}) {
    _emit(_state.copyWith(
      dropoffAddress: address,
      dropoffLat: lat,
      dropoffLng: lng,
      clearError: true,
    ));
  }

  void backToLocationEntry() {
    _emit(_state.copyWith(
      status: TaxiFlowStatus.locationEntry,
      clearError: true,
    ));
  }

  /// Checks a driver is reachable from the pickup, then loads the fare tiles.
  ///
  /// Availability is checked first deliberately: quoting a fare for a place
  /// with no drivers just wastes the passenger's time.
  Future<void> checkAvailabilityAndQuote() async {
    final token = _accessToken();
    if (token == null) {
      _requireAuth();
      return;
    }
    if (!_state.canRequestFare) {
      _emit(_state.copyWith(
        error: 'Choose a pickup and destination first.',
      ));
      return;
    }

    _emit(_state.copyWith(
      status: TaxiFlowStatus.checkingAvailability,
      isLoading: true,
      clearError: true,
    ));

    try {
      final availability = await _api.checkAvailability(
        accessToken: token,
        lat: _state.pickupLat,
        lng: _state.pickupLng,
      );
      if (!availability.available) {
        _emit(_state.copyWith(
          status: TaxiFlowStatus.unavailable,
          isLoading: false,
          availability: availability,
        ));
        return;
      }

      final tiles = await _api.estimateFare(
        pickupLat: _state.pickupLat,
        pickupLng: _state.pickupLng,
        dropoffLat: _state.dropoffLat,
        dropoffLng: _state.dropoffLng,
      );
      if (tiles.isEmpty) {
        _emit(_state.copyWith(
          status: TaxiFlowStatus.unavailable,
          isLoading: false,
          availability: availability,
          error: 'No ride options are available for this route.',
        ));
        return;
      }

      _emit(_state.copyWith(
        status: TaxiFlowStatus.fareSelection,
        isLoading: false,
        availability: availability,
        fareTiles: tiles,
        selectedClass: tiles.first,
      ));
      // Load the balance now so the confirm screen already knows it by the
      // time the passenger gets there.
      unawaited(refreshWalletBalance());
    } on ApiException catch (e) {
      _emit(_state.copyWith(
        status: TaxiFlowStatus.locationEntry,
        isLoading: false,
        error: e.message,
      ));
    }
  }

  // ─── Fare + options ────────────────────────────────────────────────────────

  void selectClass(TaxiFareTile tile) {
    _emit(_state.copyWith(selectedClass: tile, clearError: true));
  }

  void proceedToRideOptions() {
    if (_state.selectedClass == null) return;
    // A parcel collects its package and receiver detail between choosing a
    // car and confirming; a passenger ride goes straight to confirm.
    _emit(_state.copyWith(
      status: _state.rideMode.isParcel
          ? TaxiFlowStatus.parcelDetails
          : TaxiFlowStatus.rideOptions,
      clearError: true,
    ));
  }

  void backToFareSelection() {
    _emit(_state.copyWith(
      status: TaxiFlowStatus.fareSelection,
      clearError: true,
    ));
  }

  void setRideNotes(String notes) => _emit(_state.copyWith(rideNotes: notes));

  // ─── Parcel details ────────────────────────────────────────────────────────

  void setPackageDesc(String v) =>
      _emit(_state.copyWith(packageDesc: v, clearError: true));
  void setPackageType(String v) => _emit(_state.copyWith(packageType: v));
  void setPackageSize(String v) =>
      _emit(_state.copyWith(packageSize: v, clearError: true));
  void setIsFragile(bool v) => _emit(_state.copyWith(isFragile: v));
  void setReceiverName(String v) =>
      _emit(_state.copyWith(receiverName: v, clearError: true));
  void setReceiverPhone(String v) =>
      _emit(_state.copyWith(receiverPhone: v, clearError: true));

  /// Surfaces which parcel detail is still missing, rather than leaving a
  /// disabled button with no explanation.
  void showParcelDetailsIncomplete() {
    _emit(_state.copyWith(
      error: 'Add what you are sending, and the receiver\'s name and phone.',
    ));
  }

  void proceedFromParcelDetails() {
    if (!_state.canSubmitParcelDetails) return;
    _emit(_state.copyWith(
      status: TaxiFlowStatus.rideOptions,
      clearError: true,
    ));
  }

  void backToParcelDetails() {
    _emit(_state.copyWith(
      status: TaxiFlowStatus.parcelDetails,
      clearError: true,
    ));
  }

  // ─── Wallet ────────────────────────────────────────────────────────────────

  /// Exposes what the confirm view needs to launch the shared top-up flow
  /// (the funding screen wires its own controller from these).
  WalletApi get walletApi => _walletApi;
  String? get accessTokenForWallet => _accessToken();
  String get customerEmail {
    final s = _auth.state;
    return s.profileEmail.trim().isNotEmpty
        ? s.profileEmail.trim()
        : s.email.trim();
  }

  /// Re-fetches the wallet balance (on quote, and after a top-up returns).
  /// Non-fatal: a failure just leaves the balance unknown, which never blocks
  /// booking -- the authoritative check is the wallet hold on acceptance.
  Future<void> refreshWalletBalance() async {
    final token = _accessToken();
    if (token == null) return;
    try {
      final wallet = await _walletApi.getWallet(accessToken: token);
      _emit(_state.copyWith(walletBalanceKobo: wallet.availableKobo));
    } catch (_) {
      // non-fatal: balance simply will not update
    }
  }

  // ─── Booking ───────────────────────────────────────────────────────────────

  Future<void> confirmBooking() async {
    final token = _accessToken();
    if (token == null) {
      _requireAuth();
      return;
    }
    final selected = _state.selectedClass;
    if (selected == null) return;

    _emit(_state.copyWith(isLoading: true, clearError: true));
    try {
      final booking = await _api.createBooking(
        accessToken: token,
        vehicleClass: selected.vehicleClass,
        pickupAddress: _state.pickupAddress,
        pickupLat: _state.pickupLat,
        pickupLng: _state.pickupLng,
        dropoffAddress: _state.dropoffAddress,
        dropoffLat: _state.dropoffLat,
        dropoffLng: _state.dropoffLng,
        rideNotes: _state.rideNotes,
        mode: _state.rideMode,
        receiverName: _state.receiverName,
        receiverPhone: _state.receiverPhone,
        packageDesc: _state.packageDesc,
        packageType: _state.packageType,
        packageSize: _state.packageSize,
        isFragile: _state.isFragile,
      );
      _bumpGeneration();
      _emit(_state.copyWith(
        status: TaxiFlowStatus.searching,
        isLoading: false,
        activeBooking: booking,
      ));
      _startPolling();
      _startSearchTimeout();
    } on ApiException catch (e) {
      _emit(_state.copyWith(isLoading: false, error: e.message));
    }
  }

  Future<void> cancelBooking({String reason = 'customer_cancelled'}) async {
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
      _stopSearchTimeout();
      _emit(_state.copyWith(
        status: TaxiFlowStatus.cancelled,
        isLoading: false,
        activeBooking: updated,
      ));
      unawaited(loadHistory());
    } on ApiException catch (e) {
      // A cancel refused because the trip already started is a real answer,
      // not a transient failure: surface it and let the poll keep the screen
      // truthful rather than pretending the ride ended.
      _emit(_state.copyWith(isLoading: false, error: e.message));
    }
  }

  // ─── Pickup verification (passenger shows, driver verifies) ───────────────

  /// Loads the passenger's pickup code. No-ops until the driver has arrived,
  /// because the service only generates it at that moment.
  Future<void> loadPickupCode() async {
    final token = _accessToken();
    final bookingId = _state.activeBooking?.id;
    if (token == null || bookingId == null) return;
    final generation = _stateGeneration;
    try {
      final credential =
          await _api.getPickupCode(accessToken: token, bookingId: bookingId);
      if (generation != _stateGeneration || credential == null) return;
      _emit(_state.copyWith(pickupCredential: credential));
    } on ApiException {
      // Non-fatal: the code card simply stays in its loading state and the
      // next poll retries.
    }
  }

  Future<void> resendPickupCode() async {
    final token = _accessToken();
    final bookingId = _state.activeBooking?.id;
    if (token == null || bookingId == null) return;

    _emit(_state.copyWith(isResendingCode: true, clearError: true));
    final generation = _stateGeneration;
    try {
      final credential = await _api.resendPickupCode(
        accessToken: token,
        bookingId: bookingId,
      );
      if (generation != _stateGeneration) return;
      _emit(_state.copyWith(
        pickupCredential: credential,
        isResendingCode: false,
      ));
    } on ApiException catch (e) {
      if (generation != _stateGeneration) return;
      _emit(_state.copyWith(isResendingCode: false, error: e.message));
    }
  }

  // ─── Delivery code (parcel only) ───────────────────────────────────────────

  /// Fetches the receiver's drop-off code. The service mints it on this first
  /// read rather than at trip start, so its 15-minute life begins when the
  /// customer actually opens the card — near drop-off, not an hour earlier.
  Future<void> loadDeliveryCode() async {
    final token = _accessToken();
    final bookingId = _state.activeBooking?.id;
    if (token == null || bookingId == null) return;
    final generation = _stateGeneration;
    try {
      final credential = await _api.getDeliveryCode(
        accessToken: token,
        bookingId: bookingId,
      );
      if (generation != _stateGeneration || credential == null) return;
      _emit(_state.copyWith(deliveryCredential: credential));
    } on ApiException {
      // Non-fatal: the card stays in its loading state and the next poll
      // retries.
    }
  }

  Future<void> resendDeliveryCode() async {
    final token = _accessToken();
    final bookingId = _state.activeBooking?.id;
    if (token == null || bookingId == null) return;

    _emit(_state.copyWith(isResendingDeliveryCode: true, clearError: true));
    final generation = _stateGeneration;
    try {
      final credential = await _api.resendDeliveryCode(
        accessToken: token,
        bookingId: bookingId,
      );
      if (generation != _stateGeneration) return;
      _emit(_state.copyWith(
        deliveryCredential: credential,
        isResendingDeliveryCode: false,
      ));
    } on ApiException catch (e) {
      if (generation != _stateGeneration) return;
      _emit(_state.copyWith(isResendingDeliveryCode: false, error: e.message));
    }
  }

  // ─── Review ────────────────────────────────────────────────────────────────

  void setReviewRating(int rating) =>
      _emit(_state.copyWith(reviewRating: rating, clearError: true));

  void setReviewText(String text) => _emit(_state.copyWith(reviewText: text));

  void setRecommendsDriver(bool value) =>
      _emit(_state.copyWith(recommendsDriver: value));

  Future<void> submitReview() async {
    final token = _accessToken();
    final bookingId = _state.activeBooking?.id;
    if (token == null || bookingId == null) return;
    if (_state.reviewRating < 1) {
      _emit(_state.copyWith(error: 'Choose a rating first.'));
      return;
    }

    _emit(_state.copyWith(isLoading: true, clearError: true));
    try {
      final reviews = await _api.submitReview(
        accessToken: token,
        bookingId: bookingId,
        rating: _state.reviewRating,
        reviewText: _state.reviewText,
        recommendsDriver: _state.recommendsDriver,
      );
      _bumpGeneration();
      _emit(_state.copyWith(
        status: TaxiFlowStatus.completed,
        isLoading: false,
        reviews: reviews,
      ));
      unawaited(loadHistory());
    } on ApiException catch (e) {
      _emit(_state.copyWith(isLoading: false, error: e.message));
    }
  }

  void skipReview() {
    _bumpGeneration();
    _emit(_state.copyWith(status: TaxiFlowStatus.completed, clearError: true));
    unawaited(loadHistory());
  }

  // ─── History ───────────────────────────────────────────────────────────────

  Future<void> loadHistory() async {
    final token = _accessToken();
    if (token == null) return;
    try {
      final bookings = await _api.listBookings(accessToken: token);
      _emit(_state.copyWith(bookingHistory: bookings));
    } on ApiException {
      // Non-fatal — the Trips tab keeps whatever it already has.
    }
  }

  // ─── Polling ───────────────────────────────────────────────────────────────

  void _startPolling() {
    _pollTimer?.cancel();
    _pollTimer = Timer.periodic(
      const Duration(seconds: 4),
      (_) => _pollBookingStatus(),
    );
    _pollBookingStatus();
  }

  void _stopPolling() {
    _pollTimer?.cancel();
    _pollTimer = null;
  }

  /// Forces an immediate refresh (e.g. after a push notification).
  void pokeRefresh() {
    if (_pollTimer == null) return;
    _pollBookingStatus();
  }

  /// When the driver should reach the pickup and the drop-off, refreshed with
  /// the booking poll while a trip is active.
  DateTime? pickupEta;
  DateTime? dropoffEta;

  Future<void> _refreshEtas(String token, String bookingId, int generation) async {
    try {
      final (pickup, dropoff) = await _api.getEtas(accessToken: token, bookingId: bookingId);
      if (generation != _stateGeneration) return;
      pickupEta = pickup;
      dropoffEta = dropoff;
      notifyListeners();
    } on ApiException {
      // Tracking off or briefly unavailable: keep the last estimate.
    }
  }

  Future<void> _pollBookingStatus() async {
    if (_pollInFlight) return;
    final token = _accessToken();
    final bookingId = _state.activeBooking?.id;
    if (token == null || bookingId == null) return;

    _pollInFlight = true;
    final generation = _stateGeneration;
    try {
      final booking =
          await _api.getBooking(accessToken: token, bookingId: bookingId);
      if (generation != _stateGeneration) return;
      _applyBookingUpdate(booking);
      if (booking.status.isActive) {
        await _refreshEtas(token, bookingId, generation);
      } else if (pickupEta != null || dropoffEta != null) {
        pickupEta = dropoffEta = null;
        notifyListeners();
      }
    } on ApiException {
      // Transient — the next tick retries rather than tearing down the screen.
    } finally {
      _pollInFlight = false;
    }
  }

  void _applyBookingUpdate(TaxiBooking booking) {
    final status = booking.status;

    if (status.isTerminal) {
      _stopPolling();
      _stopSearchTimeout();
      if (status == TaxiBookingStatus.completed) {
        _emit(_state.copyWith(
          status: TaxiFlowStatus.review,
          activeBooking: booking,
        ));
      } else {
        _emit(_state.copyWith(
          status: TaxiFlowStatus.cancelled,
          activeBooking: booking,
          error: status == TaxiBookingStatus.unmatched
              ? 'No drivers were available. Please try again.'
              : _state.error,
        ));
      }
      unawaited(loadHistory());
      return;
    }

    if (status.isSearching) {
      _emit(_state.copyWith(
        status: TaxiFlowStatus.searching,
        activeBooking: booking,
      ));
      return;
    }

    // A driver is assigned. Stop the search timeout so a slow-but-successful
    // match is never cancelled out from under the passenger.
    _stopSearchTimeout();
    _emit(_state.copyWith(
      status: TaxiFlowStatus.activeTrip,
      activeBooking: booking,
    ));

    // The code only exists once the driver has arrived, and it is the
    // passenger who must display it.
    if (booking.pickupVerification == PickupVerification.pending &&
        _state.pickupCredential == null) {
      unawaited(loadPickupCode());
    }
    // A parcel's drop-off code becomes available once the package is moving.
    if (booking.isParcel &&
        booking.deliveryVerification == DeliveryVerification.pending &&
        _state.deliveryCredential == null) {
      unawaited(loadDeliveryCode());
    }
  }

  /// How long to wait for a driver before giving up.
  ///
  /// The service's own search window (TAXI_BOOKING_SEARCH_WINDOW) defaults to
  /// 60s, after which it marks the booking unmatched itself. This sits just
  /// past that so the server's own answer normally lands first and the client
  /// timeout is only a backstop for a service that never replies.
  static const searchTimeout = Duration(seconds: 75);

  void _startSearchTimeout() {
    _searchTimeoutTimer?.cancel();
    _searchTimeoutTimer = Timer(searchTimeout, () async {
      if (_state.status != TaxiFlowStatus.searching) return;
      await cancelBooking(reason: 'search_timeout');
      if (_state.status == TaxiFlowStatus.cancelled) {
        _emit(_state.copyWith(
          error: 'No drivers found nearby. Please try again later.',
        ));
      }
    });
  }

  void _stopSearchTimeout() {
    _searchTimeoutTimer?.cancel();
    _searchTimeoutTimer = null;
  }

  @override
  void dispose() {
    _stopPolling();
    _stopSearchTimeout();
    super.dispose();
  }
}
