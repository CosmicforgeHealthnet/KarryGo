import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/foundation.dart';

import '../../../core/app_mode.dart';
import '../../trips/data/trip_model.dart';
import '../data/rider_support_api.dart';
import '../mock/disputes_mock.dart';
import '../models/dispute_models.dart';

/// Drives the disputes flow (select trip → select type → submit), the disputes
/// list, and per-complaint live chat against support-dispute-service. Mirrors
/// the truck provider's ProviderDisputeController; dispatch riders dispute
/// against their trips (the app has no wallet transaction-history endpoint).
class RiderDisputeController extends ChangeNotifier {
  RiderDisputeController({
    required RiderSupportApi api,
    required String? Function() accessToken,
  }) : _api = api,
       _accessToken = accessToken;

  final RiderSupportApi _api;
  final String? Function() _accessToken;
  String? get _token => _accessToken();
  bool _disposed = false;
  void _notify() {
    if (!_disposed) notifyListeners();
  }

  @override
  void dispose() {
    _disposed = true;
    _messageGeneration++;
    super.dispose();
  }

  // ─── Disputes list ──────────────────────────────────────────────────────────
  List<Complaint> _complaints = const [];
  List<Complaint> get complaints => _complaints;
  bool _loading = false;
  bool get loading => _loading;
  String? _error;
  String? get error => _error;

  Future<Complaint> refreshComplaint(String id) async {
    final Complaint updated;
    if (AppModeConfig.instance.isMock) {
      updated = DisputesMock.complaints().firstWhere((c) => c.id == id);
    } else {
      final token = _token;
      if (token == null || token.isEmpty)
        throw const ApiException(
          code: ApiErrorCode.unauthorized,
          message: 'Sign in again to refresh this case.',
        );
      updated = await _api.getComplaint(accessToken: token, id: id);
    }
    if (updated.id != id)
      throw const ApiException(
        code: ApiErrorCode.unknown,
        message: 'This case could not be confirmed. Please retry.',
      );
    if (!_disposed) {
      _complaints = [for (final c in _complaints) c.id == id ? updated : c];
      _notify();
    }
    return updated;
  }

  Future<void> loadComplaints() async {
    if (_disposed || _loading) return;
    _loading = true;
    _error = null;
    _notify();

    // ── MOCK ──
    if (AppModeConfig.instance.isMock) {
      await Future.delayed(const Duration(milliseconds: 300));
      if (_disposed) return;
      _complaints = DisputesMock.complaints();
      _loading = false;
      _notify();
      return;
    }

    // ── LIVE ──
    final token = _token;
    if (token == null) {
      _loading = false;
      _notify();
      return;
    }
    try {
      _complaints = await _api.listComplaints(accessToken: token);
    } catch (e) {
      _error = _msg(e);
    } finally {
      _loading = false;
      _notify();
    }
  }

  // ─── New-dispute selection ──────────────────────────────────────────────────
  TripModel? _selectedTrip;
  TripModel? get selectedTrip => _selectedTrip;
  String? _selectedType;
  String? get selectedType => _selectedType;

  void startNewDispute() {
    _selectedTrip = null;
    _selectedType = null;
    _submitError = null;
    _createdComplaint = null;
    _notify();
  }

  void selectTrip(TripModel trip) {
    _selectedTrip = trip;
    _notify();
  }

  void selectType(String type) {
    _selectedType = type;
    _notify();
  }

  // ─── Submission ─────────────────────────────────────────────────────────────
  bool _submitting = false;
  bool get submitting => _submitting;
  String? _submitError;
  String? get submitError => _submitError;
  Complaint? _createdComplaint;
  Complaint? get createdComplaint => _createdComplaint;

  Future<Complaint?> submitDispute() async {
    final trip = _selectedTrip;
    final type = _selectedType;
    if (trip == null || type == null) return null;

    return createCase(
      subject: type,
      description: 'Dispute on trip ${trip.id}: $type',
      bookingReference: trip.bookingId.isNotEmpty ? trip.bookingId : trip.id,
    );
  }

  Future<Complaint?> createCase({
    required String subject,
    required String description,
    String bookingReference = '',
    String? category,
  }) async {
    if (_disposed || _submitting) return null;
    _submitting = true;
    _submitError = null;
    _notify();
    try {
      Complaint complaint;
      if (AppModeConfig.instance.isMock) {
        await Future<void>.delayed(const Duration(milliseconds: 300));
        if (_disposed) return null;
        complaint = DisputesMock.addComplaint(
          subject: subject,
          description: description,
          bookingReference: bookingReference,
        );
      } else {
        final token = _token;
        if (token == null || token.isEmpty)
          throw const ApiException(
            code: ApiErrorCode.unauthorized,
            message: 'Sign in again to contact support.',
          );
        complaint = await _api.createComplaint(
          accessToken: token,
          subject: subject,
          description: description,
          bookingReference: bookingReference,
          category: category,
        );
      }
      if (_disposed) return null;
      _createdComplaint = complaint;
      _complaints = [
        complaint,
        ..._complaints.where((c) => c.id != complaint.id),
      ];
      return complaint;
    } catch (e) {
      _submitError =
          (e is ApiException &&
              e.statusCode != null &&
              e.statusCode! >= 400 &&
              e.statusCode! < 500)
          ? _msg(e)
          : '${_msg(e)} Check My cases before submitting again.';
      return null;
    } finally {
      _submitting = false;
      _notify();
    }
  }

  // ─── Support live chat (profile > Support) ─────────────────────────────────
  //
  // The live chat is a regular complaint on support-dispute-service with a
  // well-known subject, so agent replies arrive through the same admin surface
  // as dispute chats. One open support complaint is reused per rider.
  static const supportChatSubject = 'Support Chat';

  Complaint? _supportComplaint;
  Complaint? get supportComplaint => _supportComplaint;
  bool _openingSupportChat = false;
  bool get openingSupportChat => _openingSupportChat;
  String? _supportChatError;
  String? get supportChatError => _supportChatError;

  /// Finds the rider's open support-chat complaint or creates one.
  /// Returns null on failure (error in [supportChatError]).
  Future<Complaint?> openSupportChat() async {
    if (_disposed || _openingSupportChat) return null;
    _openingSupportChat = true;
    _supportChatError = null;
    _notify();
    try {
      if (AppModeConfig.instance.isMock) {
        await Future<void>.delayed(const Duration(milliseconds: 200));
        if (_disposed) return null;
        _supportComplaint = DisputesMock.supportChatComplaint();
      } else {
        final token = _token;
        if (token == null || token.isEmpty)
          throw const ApiException(
            code: ApiErrorCode.unauthorized,
            message: 'Sign in again to contact support.',
          );
        _supportComplaint = await _api.startSupportChat(accessToken: token);
      }
      return _disposed ? null : _supportComplaint;
    } catch (e) {
      _supportChatError = _msg(e);
      return null;
    } finally {
      _openingSupportChat = false;
      _notify();
    }
  }

  /// Marks agent messages on a complaint as read. Failure-silent.
  Future<void> markMessagesRead(String complaintId) async {
    if (AppModeConfig.instance.isMock) return;
    final token = _token;
    if (token == null) return;
    try {
      await _api.markMessagesRead(accessToken: token, id: complaintId);
    } catch (_) {}
  }

  // ─── Live chat ──────────────────────────────────────────────────────────────
  List<DisputeMessage> _messages = const [];
  List<DisputeMessage> get messages => _messages;
  bool _loadingMessages = false;
  bool get loadingMessages => _loadingMessages;
  bool _sending = false;
  bool get sending => _sending;

  String? _messageComplaintId;
  int _messageGeneration = 0;
  String? messagesError;
  String? sendError;

  Future<void> loadMessages(String complaintId) async {
    if (_disposed || (_loadingMessages && _messageComplaintId == complaintId))
      return;
    if (_messageComplaintId != complaintId) {
      _messages = const [];
      _messageComplaintId = complaintId;
      sendError = null;
    }
    final generation = ++_messageGeneration;
    _loadingMessages = true;
    messagesError = null;
    _notify();
    try {
      List<DisputeMessage> loaded;
      if (AppModeConfig.instance.isMock) {
        await Future<void>.delayed(const Duration(milliseconds: 200));
        loaded = DisputesMock.messages(complaintId);
      } else {
        final token = _token;
        if (token == null || token.isEmpty)
          throw const ApiException(
            code: ApiErrorCode.unauthorized,
            message: 'Sign in again to load messages.',
          );
        loaded = await _api.listMessages(accessToken: token, id: complaintId);
      }
      if (_disposed || generation != _messageGeneration) return;
      final merged = {
        for (final m in _messages) m.id: m,
        for (final m in loaded) m.id: m,
      };
      _messages = merged.values.toList()
        ..sort((a, b) => a.createdAt.compareTo(b.createdAt));
    } catch (e) {
      if (!_disposed && generation == _messageGeneration)
        messagesError = _msg(e);
    } finally {
      if (!_disposed && generation == _messageGeneration) {
        _loadingMessages = false;
        _notify();
      }
    }
  }

  Future<bool> sendMessage(String complaintId, String content) async {
    if (_disposed || _sending || content.trim().isEmpty) return false;
    _sending = true;
    sendError = null;
    _notify();
    try {
      DisputeMessage msg;
      if (AppModeConfig.instance.isMock) {
        await Future<void>.delayed(const Duration(milliseconds: 200));
        if (_disposed) return false;
        msg = DisputesMock.sendMessage(complaintId, content.trim());
      } else {
        final token = _token;
        if (token == null || token.isEmpty)
          throw const ApiException(
            code: ApiErrorCode.unauthorized,
            message: 'Sign in again to send your message.',
          );
        msg = await _api.sendMessage(
          accessToken: token,
          id: complaintId,
          content: content.trim(),
        );
      }
      if (_disposed) return false;
      if (_messageComplaintId == complaintId)
        _messages = [..._messages.where((m) => m.id != msg.id), msg];
      return true;
    } catch (e) {
      if (_messageComplaintId == complaintId) sendError = _msg(e);
      return false;
    } finally {
      _sending = false;
      _notify();
    }
  }

  String _msg(Object e) => e is ApiException ? e.message : e.toString();
}
