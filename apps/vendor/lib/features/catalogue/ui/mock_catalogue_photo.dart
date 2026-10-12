import 'package:flutter/material.dart';

class MockCataloguePhoto extends StatelessWidget {
  const MockCataloguePhoto({
    super.key,
    required this.name,
    required this.id,
    this.size = 64,
  });
  final String name, id;
  final double size;

  String get asset {
    final label = name.toLowerCase();
    if (label.contains('jollof') ||
        label.contains('rice') ||
        label.contains('favourite')) {
      return 'assets/onboarding/menu-banner-meal.png';
    }
    if (label.contains('burger')) return 'assets/catalogue/burger.jpg';
    if (label.contains('pizza')) return 'assets/catalogue/pizza.jpg';
    if (label.contains('salad') ||
        label.contains('grocery') ||
        label.contains('vegetable')) {
      return 'assets/catalogue/salad.jpg';
    }
    const photos = ['pizza', 'burger', 'salad'];
    final index =
        id.codeUnits.fold<int>(0, (sum, value) => sum + value) % photos.length;
    return 'assets/catalogue/${photos[index]}.jpg';
  }

  @override
  Widget build(BuildContext context) => ExcludeSemantics(
    child: ClipRRect(
      borderRadius: BorderRadius.circular(16),
      child: Image.asset(asset, width: size, height: size, fit: BoxFit.cover),
    ),
  );
}
