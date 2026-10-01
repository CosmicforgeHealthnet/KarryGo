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

  // ─── Disputes list ──────────────────────────────────────────────────────────
  List<Complaint> _complaints = const [];
  List<Complaint> get complaints => _complaints;
  bool _loading = false;
  bool get loading => _loading;
  String? _error;
  String? get error => _error;

  Future<void> loadComplaints() async {
    _loading = true;
    _error = null;
    notifyListeners();

    // ── MOCK ──
    if (AppModeConfig.instance.isMock) {
      await Future.delayed(const Duration(milliseconds: 300));
      _complaints = DisputesMock.complaints();
      _loading = false;
      notifyListeners();
      return;
    }

    // ── LIVE ──
    final token = _token;
    if (token == null) {
      _loading = false;
      notifyListeners();
      return;
    }
    try {
      _complaints = await _api.listComplaints(accessToken: token);
    } catch (e) {
      _error = _msg(e);
    } finally {
      _loading = false;
      notifyListeners();
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
    notifyListeners();
  }

  void selectTrip(TripModel trip) {
    _selectedTrip = trip;
    notifyListeners();
  }

  void selectType(String type) {
    _selectedType = type;
    notifyListeners();
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

    _submitting = true;
    _submitError = null;
    notifyListeners();

    // ── MOCK ──
    if (AppModeConfig.instance.isMock) {
      await Future.delayed(const Duration(milliseconds: 600));
      final reference = trip.bookingId.isNotEmpty ? trip.bookingId : trip.id;
      final complaint = DisputesMock.addComplaint(
        subject: type,
        description: 'Dispute on trip ${trip.id}: $type',
        bookingReference: reference,
      );
      _createdComplaint = complaint;
      _complaints = [complaint, ..._complaints];
      _submitting = false;
      notifyListeners();
      return complaint;
    }

    // ── LIVE ──
    final token = _token;
    if (token == null) {
      _submitting = false;
      notifyListeners();
      return null;
    }

    try {
      final reference = trip.bookingId.isNotEmpty ? trip.bookingId : trip.id;
      final complaint = await _api.createComplaint(
        accessToken: token,
        subject: type,
        description: 'Dispute on trip ${trip.id}: $type',
        bookingReference: reference,
      );
      _createdComplaint = complaint;
      _complaints = [complaint, ..._complaints];
      return complaint;
    } catch (e) {
      _submitError = _msg(e);
      return null;
    } finally {
      _submitting = false;
      notifyListeners();
    }
  }

  // ─── Support live chat (profile > Support) ─────────────────────────────────
  //
  // The live chat is a regular complaint on support-dispute-service with a
  // well-known subject, so agent replies arrive through the same admin surface
  // as dispute chats. One open support complaint is reused per rider.
  static const supportChatSubject = 'Live chat support';

  Complaint? _supportComplaint;
  Complaint? get supportComplaint => _supportComplaint;
  bool _openingSupportChat = false;
  bool get openingSupportChat => _openingSupportChat;
  String? _supportChatError;
  String? get supportChatError => _supportChatError;

  /// Finds the rider's open support-chat complaint or creates one.
  /// Returns null on failure (error in [supportChatError]).
  Future<Complaint?> openSupportChat() async {
    _openingSupportChat = true;
    _supportChatError = null;
    notifyListeners();

    // ── MOCK ──
    if (AppModeConfig.instance.isMock) {
      await Future.delayed(const Duration(milliseconds: 400));
      final c = DisputesMock.supportChatComplaint();
      _supportComplaint = c;
      _openingSupportChat = false;
      notifyListeners();
      return c;
    }

    // ── LIVE ──
    final token = _token;
    if (token == null) {
      _openingSupportChat = false;
      notifyListeners();
      return null;
    }
    try {
      final complaints = await _api.listComplaints(accessToken: token);
      Complaint? existing;
      for (final c in complaints) {
        final closed = c.status == 'resolved' || c.status == 'closed';
        if (c.subject == supportChatSubject && !closed) {
          existing = c;
          break;
        }
      }
      _supportComplaint =
          existing ??
          await _api.createComplaint(
            accessToken: token,
            subject: supportChatSubject,
            description: 'Live chat conversation with support.',
            bookingReference: '',
          );
      return _supportComplaint;
    } catch (e) {
      _supportChatError = _msg(e);
      return null;
    } finally {
      _openingSupportChat = false;
      notifyListeners();
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

  Future<void> loadMessages(String complaintId) async {
    // ── MOCK ──
    if (AppModeConfig.instance.isMock) {
      await Future.delayed(const Duration(milliseconds: 200));
      _messages = DisputesMock.messages(complaintId);
      notifyListeners();
      return;
    }

    // ── LIVE ──
    final token = _token;
    if (token == null) return;
    _loadingMessages = true;
    notifyListeners();
    try {
      _messages = await _api.listMessages(accessToken: token, id: complaintId);
    } catch (_) {
      // Leave existing messages; the chat screen still renders the composer.
    } finally {
      _loadingMessages = false;
      notifyListeners();
    }
  }

  Future<void> sendMessage(String complaintId, String content) async {
    if (content.trim().isEmpty) return;

    // ── MOCK ──
    if (AppModeConfig.instance.isMock) {
      await Future.delayed(const Duration(milliseconds: 400));
      final msg = DisputesMock.sendMessage(complaintId, content.trim());
      _messages = [..._messages, msg];
      notifyListeners();
      return;
    }

    // ── LIVE ──
    final token = _token;
    if (token == null) return;
    _sending = true;
    notifyListeners();
    try {
      final msg = await _api.sendMessage(
        accessToken: token,
        id: complaintId,
        content: content.trim(),
      );
      _messages = [..._messages, msg];
    } catch (_) {
    } finally {
      _sending = false;
      notifyListeners();
    }
  }

  String _msg(Object e) => e is ApiException ? e.message : e.toString();
}