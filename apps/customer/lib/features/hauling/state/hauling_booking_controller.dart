import 'dart:async';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/foundation.dart';

import '../../auth/state/customer_auth_controller.dart';
import '../../wallet/data/wallet_api.dart';
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
  tierSelection,
  payment,
  paymentProcessing,
  searching,
  activeTrip,
  delivered,
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
    // location inputs
    this.pickupAddress = '',
    this.pickupLat = 0.0,
    this.pickupLng = 0.0,
    this.dropoffAddress = '',
    this.dropoffLat = 0.0,
    this.dropoffLng = 0.0,
    // tier + form selections
    this.selectedTier,
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
    this.paystackUrl,
    // fare + booking
    this.fareEstimate,
    this.activeBooking,
    this.bookingHistory = const [],
    // provider snapshot (loaded when booking is accepted)
    this.providerSnapshot,
    this.truckSnapshot,
  });

  const HaulingBookingState.idle() : this(status: HaulingFlowStatus.idle);

  final HaulingFlowStatus status;
  final bool isLoading;
  final String? error;
  final AvailabilityResult? availability;

  final String pickupAddress;
  final double pickupLat;
  final double pickupLng;
  final String dropoffAddress;
  final double dropoffLat;
  final double dropoffLng;

  final TruckTier? selectedTier;
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
  final String? paystackUrl;

  final FareEstimate? fareEstimate;
  final HaulageBooking? activeBooking;
  final List<HaulageBooking> bookingHistory;

  final ProviderSnapshot? providerSnapshot;
  final TruckSnapshot? truckSnapshot;

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
    String? pickupAddress,
    double? pickupLat,
    double? pickupLng,
    String? dropoffAddress,
    double? dropoffLat,
    double? dropoffLng,
    TruckTier? selectedTier,
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
    String? paystackUrl,
    FareEstimate? fareEstimate,
    HaulageBooking? activeBooking,
    List<HaulageBooking>? bookingHistory,
    ProviderSnapshot? providerSnapshot,
    TruckSnapshot? truckSnapshot,
    bool clearError = false,
    bool clearFare = false,
    bool clearBooking = false,
    bool clearScheduledAt = false,
    bool clearPaystackUrl = false,
    bool clearProviderSnapshot = false,
    bool clearRecommendsDriver = false,
  }) {
    return HaulingBookingState(
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
      selectedTier: selectedTier ?? this.selectedTier,
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
      paystackUrl: clearPaystackUrl ? null : (paystackUrl ?? this.paystackUrl),
      fareEstimate: clearFare ? null : (fareEstimate ?? this.fareEstimate),
      activeBooking: clearBooking ? null : (activeBooking ?? this.activeBooking),
      bookingHistory: bookingHistory ?? this.bookingHistory,
      providerSnapshot: clearProviderSnapshot ? null : (providerSnapshot ?? this.providerSnapshot),
      truckSnapshot: clearProviderSnapshot ? null : (truckSnapshot ?? this.truckSnapshot),
    );
  }
}

// ─── Controller ───────────────────────────────────────────────────────────────

class HaulingBookingController extends ChangeNotifier {
  HaulingBookingController({
    required HaulingApi api,
    required CustomerAuthController authController,
    required WalletApi walletApi,
  })  : _api = api,
        _auth = authController,
        _walletApi = walletApi;

  final HaulingApi _api;
  final CustomerAuthController _auth;
  final WalletApi _walletApi;
  Timer? _pollTimer;

  HaulingBookingState _state = const HaulingBookingState.idle();
  HaulingBookingState get state => _state;

  void _emit(HaulingBookingState next) {
    _state = next;
    notifyListeners();
  }

  String? _accessToken() => _auth.state.session?.accessToken;

  // ─── Flow entry ────────────────────────────────────────────────────────────

  void startHaulingFlow() {
    _emit(_state.copyWith(
      status: HaulingFlowStatus.details,
      clearError: true,
    ));
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
          status: HaulingFlowStatus.tierSelection,
          isLoading: false,
          availability: result,
        ));
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

  // ─── Tier selection ────────────────────────────────────────────────────────

  void selectTier(TruckTier tier) {
    _emit(_state.copyWith(selectedTier: tier));
  }

  void proceedFromDetailsToPackageInfo() {
    _emit(_state.copyWith(status: HaulingFlowStatus.packageInfo, clearError: true));
  }

  void proceedFromPackageInfoToLocation() {
    _emit(_state.copyWith(status: HaulingFlowStatus.locationEntry, clearError: true));
  }

  void proceedFromTierToPayment() {
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
      _emit(_state.copyWith(
        status: HaulingFlowStatus.completed,
        isLoading: false,
      ));
    } catch (e) {
      _emit(_state.copyWith(isLoading: false, error: _errorMessage(e)));
    }
  }

  void skipReview() {
    _emit(_state.copyWith(status: HaulingFlowStatus.completed));
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
      ));
    } catch (e) {
      _emit(_state.copyWith(isLoading: false, error: _errorMessage(e)));
    }
  }

  void setPaymentMethod(String method) {
    _emit(_state.copyWith(paymentMethod: method, clearError: true));
  }

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
  }

  void _stopPolling() {
    _pollTimer?.cancel();
    _pollTimer = null;
  }

  Future<void> _pollBookingStatus() async {
    final token = _accessToken();
    final bookingId = _state.activeBooking?.id;
    if (token == null || bookingId == null) return;

    try {
      final booking = await _api.getBooking(accessToken: token, bookingId: bookingId);
      await _applyBookingUpdate(booking);
    } catch (_) {
      // network hiccup — keep polling
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
      return;
    }

    if (status == HaulingBookingStatus.delivered) {
      _stopPolling();
      _emit(_state.copyWith(status: HaulingFlowStatus.review, activeBooking: booking));
      return;
    }

    if (status.isActive) {
      // When a provider is assigned, load their snapshot if not yet loaded.
      if (booking.providerId != null &&
          booking.truckId != null &&
          _state.providerSnapshot?.id != booking.providerId) {
        _emit(_state.copyWith(status: HaulingFlowStatus.activeTrip, activeBooking: booking));
        _fetchProviderAndTruckInfo(booking.providerId!, booking.truckId!);
      } else {
        _emit(_state.copyWith(status: HaulingFlowStatus.activeTrip, activeBooking: booking));
      }
      return;
    }

    // searching: pendingMatch or awaitingAcceptance
    // Load provider snapshot when we know the provider (awaitingAcceptance).
    if (booking.providerId != null &&
        booking.truckId != null &&
        _state.providerSnapshot?.id != booking.providerId) {
      _emit(_state.copyWith(activeBooking: booking));
      _fetchProviderAndTruckInfo(booking.providerId!, booking.truckId!);
    } else {
      _emit(_state.copyWith(activeBooking: booking));
    }
  }

  void _fetchProviderAndTruckInfo(String providerId, String truckId) {
    final token = _accessToken();
    if (token == null) return;

    _api.getProvider(accessToken: token, providerId: providerId).then((p) {
      _emit(_state.copyWith(providerSnapshot: p));
    }).catchError((_) {});

    _api.getTruck(accessToken: token, truckId: truckId).then((t) {
      _emit(_state.copyWith(truckSnapshot: t));
    }).catchError((_) {});
  }

  // ─── Navigation helpers ────────────────────────────────────────────────────

  void reset() {
    _stopPolling();
    _emit(const HaulingBookingState.idle());
  }

  void backToDetails() {
    _emit(_state.copyWith(status: HaulingFlowStatus.details, clearFare: true));
  }

  void backToPackageInfo() {
    _emit(_state.copyWith(status: HaulingFlowStatus.packageInfo, clearError: true));
  }

  void backToPayment() {
    _emit(_state.copyWith(status: HaulingFlowStatus.payment, clearError: true));
  }

  void backToLocationEntry() {
    _emit(_state.copyWith(status: HaulingFlowStatus.locationEntry, clearError: true));
  }

  void backToTierSelection() {
    _emit(_state.copyWith(status: HaulingFlowStatus.tierSelection, clearError: true));
  }

  // ─── Utilities ─────────────────────────────────────────────────────────────

  String _errorMessage(Object e) {
    if (e is ApiException) return e.message;
    return e.toString();
  }

  @override
  void dispose() {
    _stopPolling();
    super.dispose();
  }
}
