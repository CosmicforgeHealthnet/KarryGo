import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/foundation.dart';
import 'package:image_picker/image_picker.dart';

import '../../auth/models/provider_auth_models.dart';
import '../../auth/state/provider_auth_controller.dart';
import '../../media/data/media_upload_service.dart';
import '../data/provider_profile_api.dart';

/// Owns the provider profile + trucks for the Profile section. Loads on demand,
/// persists edits through [ProviderProfileApi], and pushes name/photo/phone
/// changes back into the auth session via [ProviderAuthController].
class ProviderProfileController extends ChangeNotifier {
  ProviderProfileController({
    required ProviderProfileApi api,
    required ProviderAuthController authController,
    required MediaUploadService mediaUploadService,
  })  : _api = api,
        _auth = authController,
        _media = mediaUploadService;

  final ProviderProfileApi _api;
  final ProviderAuthController _auth;
  final MediaUploadService _media;

  bool _loading = false;
  bool get loading => _loading;

  String? _error;
  String? get error => _error;

  TruckProvider? _profile;
  TruckProvider? get profile => _profile ?? _auth.state.session?.provider;

  List<ProviderTruck> _trucks = const [];
  List<ProviderTruck> get trucks => _trucks;

  bool _trucksLoading = false;
  bool get trucksLoading => _trucksLoading;

  // ── Phone-change transient state ──
  String? _phoneChallengeId;
  String _pendingPhone = '';
  String? _debugOtp;
  String get pendingPhone => _pendingPhone;
  String? get debugOtp => _debugOtp;

  String? get _token => _auth.state.session?.accessToken;
  String get _providerId => _auth.state.session?.provider.id ?? '';

  // ─── Loading ────────────────────────────────────────────────────────────────

  Future<void> load() async {
    final token = _token;
    if (token == null) return;
    _loading = true;
    _error = null;
    notifyListeners();
    try {
      final profile = await _api.getProfile(accessToken: token);
      _profile = profile;
      _auth.applyProviderUpdate(profile);
    } catch (e) {
      _error = _msg(e);
    } finally {
      _loading = false;
      notifyListeners();
    }
    await loadTrucks();
  }

  Future<void> loadTrucks() async {
    final token = _token;
    if (token == null) return;
    _trucksLoading = true;
    notifyListeners();
    try {
      _trucks = await _api.listTrucks(accessToken: token);
    } catch (_) {
      // keep existing list on transient failure
    } finally {
      _trucksLoading = false;
      notifyListeners();
    }
  }

  // ─── Profile info ───────────────────────────────────────────────────────────

  Future<bool> saveProfileInfo({
    required String firstName,
    required String lastName,
    required String email,
    required String locationState,
    required String locationCity,
    required String language,
  }) {
    return _runUpdate(() => _api.updateProfile(
          accessToken: _token!,
          firstName: firstName,
          lastName: lastName,
          email: email,
          locationState: locationState,
          locationCity: locationCity,
          language: language,
        ));
  }

  Future<bool> updateProfilePhoto(XFile file) async {
    final token = _token;
    if (token == null) return false;
    return _runUpdate(() async {
      final uploaded = await _media.uploadPicked(
        ownerId: _providerId,
        purpose: MediaPurpose.profilePhoto,
        file: file,
      );
      return _api.updateProfile(
        accessToken: token,
        firstName: profile?.firstName ?? '',
        lastName: profile?.lastName ?? '',
        profilePhotoUrl: uploaded.url,
        photoAssetId: uploaded.id,
      );
    });
  }

  // ─── Verification & documents ────────────────────────────────────────────────

  /// Uploads a picked document image and returns its stored URL. Throws on
  /// failure so the caller can surface it inline.
  Future<String?> uploadDocument(XFile file) async {
    final result = await _media.uploadPicked(
      ownerId: _providerId,
      purpose: MediaPurpose.documentFile,
      file: file,
    );
    return result.url;
  }

  Future<bool> saveVerification({
    required String licenseNumber,
    required String expiryYear,
    required String expiryDate,
    String govIdUrl = '',
    String driverLicenseUrl = '',
    String vehicleRegUrl = '',
  }) {
    return _runUpdate(() => _api.updateProfile(
          accessToken: _token!,
          firstName: profile?.firstName ?? '',
          lastName: profile?.lastName ?? '',
          driverLicenseNumber: licenseNumber,
          licenseExpiryYear: expiryYear,
          licenseExpiryDate: expiryDate,
          govIdUrl: govIdUrl,
          driverLicenseUrl: driverLicenseUrl,
          vehicleRegUrl: vehicleRegUrl,
          submitForVerification: true,
        ));
  }

  // ─── Trucks ──────────────────────────────────────────────────────────────────

  Future<bool> saveTruck({
    String? truckId,
    required String truckType,
    required int capacityKg,
    required String plateNumber,
    required String licenseType,
    required String make,
    required String model,
    required String color,
    required String numberOfAxles,
    required String yearsOfExperience,
    required List<String> goodsTypes,
    required bool hasInsurance,
  }) async {
    final token = _token;
    if (token == null) return false;
    _error = null;
    notifyListeners();
    final body = <String, dynamic>{
      'truck_type': truckType,
      'capacity_kg': capacityKg,
      'plate_number': plateNumber,
      'license_type': licenseType,
      'make': make,
      'model': model,
      'color': color,
      'number_of_axles': numberOfAxles,
      'years_of_experience': yearsOfExperience,
      'goods_types': goodsTypes,
      'has_insurance': hasInsurance,
    };
    try {
      if (truckId == null) {
        await _api.createTruck(accessToken: token, body: body);
      } else {
        await _api.updateTruck(accessToken: token, id: truckId, body: {...body, 'status': 'active'});
      }
      await loadTrucks();
      return true;
    } catch (e) {
      _error = _msg(e);
      notifyListeners();
      return false;
    }
  }

  // ─── Phone change ─────────────────────────────────────────────────────────────

  Future<bool> startPhoneChange(String newPhone) async {
    final token = _token;
    if (token == null) return false;
    _error = null;
    _debugOtp = null;
    notifyListeners();
    try {
      final challenge = await _api.changePhoneStart(accessToken: token, phone: newPhone);
      _phoneChallengeId = challenge.challengeId;
      _pendingPhone = newPhone;
      _debugOtp = challenge.debugOtp;
      notifyListeners();
      return true;
    } catch (e) {
      _error = _msg(e);
      notifyListeners();
      return false;
    }
  }

  Future<bool> verifyPhoneChange(String otp) async {
    final token = _token;
    final challengeId = _phoneChallengeId;
    if (token == null || challengeId == null) return false;
    _error = null;
    notifyListeners();
    try {
      final updated = await _api.changePhoneVerify(
        accessToken: token,
        phone: _pendingPhone,
        otp: otp,
        challengeId: challengeId,
      );
      _profile = updated;
      _auth.applyProviderUpdate(updated);
      _phoneChallengeId = null;
      _pendingPhone = '';
      _debugOtp = null;
      notifyListeners();
      return true;
    } catch (e) {
      _error = _msg(e);
      notifyListeners();
      return false;
    }
  }

  // ─── helpers ────────────────────────────────────────────────────────────────

  Future<bool> _runUpdate(Future<TruckProvider> Function() action) async {
    if (_token == null) return false;
    _error = null;
    notifyListeners();
    try {
      final updated = await action();
      _profile = updated;
      _auth.applyProviderUpdate(updated);
      notifyListeners();
      return true;
    } catch (e) {
      _error = _msg(e);
      notifyListeners();
      return false;
    }
  }

  String _msg(Object e) {
    if (e is ApiException) {
      if (e.fields.isNotEmpty) return e.fields.map((f) => f.message).join(' ');
      return e.message;
    }
    return 'Something went wrong. Please try again.';
  }
}
