// splash screen
import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

class SplashScreen extends StatefulWidget {
  const SplashScreen({super.key, required this.onDone});
  final VoidCallback onDone;

  @override
  State<SplashScreen> createState() => _SplashScreenState();
}

class _SplashScreenState extends State<SplashScreen> {
  static const _duration = Duration(seconds: 2);

  // Assets
  static const _logoAsset = 'assets/figma/splash_logo.png';
  static const _sceneAsset = 'assets/figma/splash_scene.jpg';

  // Colors (swap for DispatchColors tokens if you want them centralised)
  static const _sceneTop = Colors.white; // matches the top of splash_scene.jpg
  static const _teal = Color(0xFF1FB5A6);
  static const _progressTrack = Color(0xFF2E5C97);
  static const _wordmark = Color(0xFF0B1F5C);
  static const _subtitle = Color(0xFF5A6B8C);

  // Layout, as a fraction of screen height
  static const _logoTop = 0.16;

  Timer? _timer;
  bool _precached = false;

  @override
  void initState() {
    super.initState();
    _timer = Timer(_duration, widget.onDone);
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (_precached) return;
    _precached = true;
    for (final path in const [_logoAsset, _sceneAsset]) {
      precacheImage(AssetImage(path), context);
    }
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final size = MediaQuery.sizeOf(context);
    final padding = MediaQuery.paddingOf(context);

    return AnnotatedRegion<SystemUiOverlayStyle>(
      value: const SystemUiOverlayStyle(
        statusBarColor: Colors.transparent,
        statusBarIconBrightness: Brightness.dark, // Android
        statusBarBrightness: Brightness.light, // iOS (light = dark icons)
        systemNavigationBarColor: Colors.transparent,
        systemNavigationBarIconBrightness: Brightness.light,
        systemNavigationBarContrastEnforced: false,
      ),
      child: Scaffold(
        backgroundColor: _sceneTop,
        body: Stack(
          children: [
            // 1. One scene image: sky, city, road and rider already blended
            Positioned.fill(
              child: Image.asset(
                _sceneAsset,
                fit: BoxFit.cover,
                alignment: Alignment.center,
              ),
            ),

            // 2. Logo: K mark (image) + wordmark and subtitle (text)
            Positioned(
              top: size.height * _logoTop,
              left: 0,
              right: 0,
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Image.asset(_logoAsset, width: size.width * 0.36),
                  const SizedBox(height: 2),
                  const Text(
                    'Karryam',
                    style: TextStyle(
                      color: _wordmark,
                      fontSize: 40,
                      fontWeight: FontWeight.w700,
                      letterSpacing: -0.5,
                      height: 0.85,
                    ),
                  ),
                  const SizedBox(height: 12),
                  const Text(
                    'DISPATCH PROVIDER',
                    style: TextStyle(
                      color: _subtitle,
                      fontSize: 11,
                      fontWeight: FontWeight.w500,
                      letterSpacing: 3,
                    ),
                  ),
                ],
              ),
            ),

            // 3. Tagline + progress
            Positioned(
              left: 28,
              right: 28,
              bottom: padding.bottom + 28,
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Text(
                    'Movement intelligence\nfor a brighter tomorrow.',
                    style: TextStyle(
                      color: Colors.white,
                      fontSize: 18,
                      fontWeight: FontWeight.w400,
                      height: 1.35,
                    ),
                  ),
                  const SizedBox(height: 16),
                  TweenAnimationBuilder<double>(
                    tween: Tween(begin: 0.4, end: 1.0),
                    duration: _duration,
                    curve: Curves.easeInOut,
                    builder: (_, value, __) => ClipRRect(
                      borderRadius: BorderRadius.circular(2),
                      child: SizedBox(
                        width: 112,
                        height: 4,
                        child: Stack(
                          children: [
                            const Positioned.fill(
                              child: ColoredBox(color: _progressTrack),
                            ),
                            FractionallySizedBox(
                              widthFactor: value,
                              alignment: Alignment.centerLeft,
                              child: const ColoredBox(color: _teal),
                            ),
                          ],
                        ),
                      ),
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}