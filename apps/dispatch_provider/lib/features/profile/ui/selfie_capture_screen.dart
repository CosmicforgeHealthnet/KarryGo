import 'dart:async';

import 'package:camera/camera.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../verification/state/verification_controller.dart';
import 'selfie_image_quality.dart';
import 'selfie_review_screen.dart';

/// Captures an actual image from the front camera, with a real preview under
/// the face guide. This is a still-image capture, NOT a liveness assertion.
/// Biometric/liveness decisions must come from the configured verification
/// provider and the authoritative backend.
class SelfieCaptureScreen extends StatefulWidget {
  const SelfieCaptureScreen({
    super.key,
    required this.verificationController,
  });

  final VerificationController verificationController;

  @override
  State<SelfieCaptureScreen> createState() => _SelfieCaptureScreenState();
}

class _SelfieCaptureScreenState extends State<SelfieCaptureScreen>
    with WidgetsBindingObserver {
  CameraController? _camera;
  bool _initializing = true;
  bool _busy = false;
  String? _cameraError;
  int _generation = 0;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    unawaited(_openFrontCamera());
  }

  Future<void> _openFrontCamera() async {
    final generation = ++_generation;
    final previous = _camera;
    _camera = null;
    if (mounted) {
      setState(() {
        _initializing = true;
        _cameraError = null;
      });
    }
    if (previous != null) await previous.dispose();

    CameraController? next;
    try {
      final cameras = await availableCameras();
      final frontCameras = cameras.where(
        (camera) => camera.lensDirection == CameraLensDirection.front,
      );
      if (frontCameras.isEmpty) {
        throw StateError('No front camera is available on this device.');
      }

      next = CameraController(
        frontCameras.first,
        ResolutionPreset.medium,
        enableAudio: false,
      );
      await next.initialize();
      if (!mounted || generation != _generation) {
        await next.dispose();
        return;
      }
      setState(() {
        _camera = next;
        _initializing = false;
        _cameraError = null;
      });
    } catch (error) {
      await next?.dispose();
      if (!mounted || generation != _generation) return;
      final message = error is CameraException &&
              (error.code == 'CameraAccessDenied' ||
                  error.code == 'CameraAccessDeniedWithoutPrompt')
          ? 'Camera access is off. Allow Karryam to use your camera in '
              'phone Settings, then try again.'
          : 'The front camera is unavailable. Close other camera apps '
              'and try again.';
      setState(() {
        _initializing = false;
        _cameraError = message;
      });
    }
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.inactive ||
        state == AppLifecycleState.paused) {
      ++_generation;
      final previous = _camera;
      _camera = null;
      if (previous != null) unawaited(previous.dispose());
    } else if (state == AppLifecycleState.resumed && !_busy) {
      unawaited(_openFrontCamera());
    }
  }

  Future<void> _capture() async {
    final camera = _camera;
    if (_busy ||
        camera == null ||
        !camera.value.isInitialized ||
        camera.value.isTakingPicture) {
      return;
    }
    final generation = _generation;
    setState(() => _busy = true);
    try {
      final captured = await camera.takePicture();
      if (!mounted || generation != _generation) return;

      final quality = await SelfieImageQuality.check(captured.path);
      if (!mounted || generation != _generation) return;
      if (!quality.isUsable) {
        _showMessage(quality.problem!);
        return;
      }

      final approved = await Navigator.of(context).push<bool>(
        MaterialPageRoute(
          builder: (_) => SelfieReviewScreen(
            verificationController: widget.verificationController,
            selfieFilePath: captured.path,
          ),
        ),
      );
      if (!mounted) return;
      if (approved == true) Navigator.of(context).pop(true);
    } on CameraException {
      if (mounted) {
        _showMessage('Could not capture a selfie. Try reopening the camera.');
      }
    } catch (_) {
      if (mounted) _showMessage('Unable to read this photo. Please retake it.');
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  void _showMessage(String message) {
    ScaffoldMessenger.of(context)
      ..hideCurrentSnackBar()
      ..showSnackBar(SnackBar(content: Text(message)));
  }

  Widget _cameraViewport() {
    final camera = _camera;
    if (_cameraError != null) {
      return Center(
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 32),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(Icons.no_photography_outlined,
                  size: 56, color: Colors.white70),
              const SizedBox(height: 16),
              Text(
                _cameraError!,
                textAlign: TextAlign.center,
                style: const TextStyle(color: Colors.white),
              ),
              const SizedBox(height: 18),
              FilledButton(
                onPressed: _busy ? null : _openFrontCamera,
                child: const Text('Try again'),
              ),
            ],
          ),
        ),
      );
    }
    if (_initializing ||
        camera == null ||
        !camera.value.isInitialized) {
      return const Center(
        child: CircularProgressIndicator(color: Colors.white),
      );
    }
    return Positioned.fill(
      child: ClipRect(
        child: CameraPreview(camera),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final cameraReady = _camera?.value.isInitialized == true &&
        !_initializing &&
        _cameraError == null &&
        !_busy;
    final guideSize =
        (MediaQuery.sizeOf(context).width - 40).clamp(200.0, 320.0).toDouble();

    return AnnotatedRegion<SystemUiOverlayStyle>(
      value: SystemUiOverlayStyle.light,
      child: Scaffold(
        backgroundColor: Colors.black,
        body: SafeArea(
          child: Column(
            children: [
              Align(
                alignment: Alignment.centerLeft,
                child: TextButton(
                  onPressed: _busy ? null : () => Navigator.of(context).pop(false),
                  child: const Text(
                    'Cancel',
                    style: TextStyle(color: Colors.white, fontSize: 16),
                  ),
                ),
              ),
              Expanded(
                child: Stack(
                  alignment: Alignment.center,
                  children: [
                    if (_cameraError == null &&
                        !_initializing &&
                        _camera?.value.isInitialized == true)
                      Positioned.fill(child: CameraPreview(_camera!))
                    else
                      _cameraViewport(),
                    // The guide overlays a live image, not an empty icon.
                    if (cameraReady)
                      IgnorePointer(
                        child: Container(
                          width: guideSize,
                          height: guideSize,
                          decoration: BoxDecoration(
                            shape: BoxShape.circle,
                            border: Border.all(
                              color: Colors.white,
                              width: 3,
                            ),
                          ),
                        ),
                      ),
                    if (cameraReady)
                      const Positioned(
                        bottom: 24,
                        left: 20,
                        right: 20,
                        child: Text(
                          'Center your face in the circle and hold still.',
                          textAlign: TextAlign.center,
                          style: TextStyle(
                            color: Colors.white,
                            fontSize: 16,
                            shadows: [
                              Shadow(blurRadius: 7, color: Colors.black),
                            ],
                          ),
                        ),
                      ),
                  ],
                ),
              ),
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 24, 20, 32),
                child: Semantics(
                  button: true,
                  label: 'Take selfie',
                  child: GestureDetector(
                    onTap: cameraReady ? _capture : null,
                    child: Container(
                      width: 76,
                      height: 76,
                      decoration: BoxDecoration(
                        shape: BoxShape.circle,
                        color: cameraReady ? Colors.white : Colors.white38,
                      ),
                      alignment: Alignment.center,
                      child: _busy
                          ? const CircularProgressIndicator(
                              color: Colors.black,
                            )
                          : const Icon(
                              Icons.camera_alt,
                              color: Colors.black87,
                            ),
                    ),
                  ),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    ++_generation;
    final previous = _camera;
    _camera = null;
    if (previous != null) unawaited(previous.dispose());
    super.dispose();
  }
}
