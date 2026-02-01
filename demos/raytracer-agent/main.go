package main

import (
	"fmt"
	"math"
	"math/rand"
	"os"
)

// rayColor calculates the color of a ray by tracing it through the scene
func rayColor(r Ray, world Hittable, depth int) Vec3 {
	// If we've exceeded the ray bounce limit, no more light is gathered
	if depth <= 0 {
		return Vec3{0, 0, 0}
	}

	// Check if the ray hits anything in the world
	if rec, hit := world.Hit(r, 0.001, math.MaxFloat64); hit {
		// Cast the material to the Material interface
		mat, ok := rec.Mat.(Material)
		if !ok {
			// If material is not a Material interface, return black
			return Vec3{0, 0, 0}
		}

		// Try to scatter the ray with the material
		if scattered, attenuation, scatOk := mat.Scatter(r, rec); scatOk {
			// Recursively calculate the color of the scattered ray
			return attenuation.MulVec(rayColor(scattered, world, depth-1))
		}

		// Ray was absorbed, return black
		return Vec3{0, 0, 0}
	}

	// No hit, use background gradient (white to sky blue)
	unitDir := r.Direction.UnitVector()
	t := 0.5 * (unitDir.Y + 1.0)
	// Linear interpolation: (1-t)*white + t*skyBlue
	white := Vec3{1.0, 1.0, 1.0}
	skyBlue := Vec3{0.5, 0.7, 1.0}
	return white.Scale(1 - t).Add(skyBlue.Scale(t))
}

func main() {
	// Image parameters
	const (
		imageWidth      = 800
		imageHeight     = 450
		samplesPerPixel = 100
		maxDepth        = 50
	)

	aspectRatio := float64(imageWidth) / float64(imageHeight)

	// Camera parameters
	camera := NewCamera(
		Vec3{13, 2, 3},   // lookFrom
		Vec3{0, 0, 0},    // lookAt
		Vec3{0, 1, 0},    // vUp
		20,               // vFOV in degrees
		aspectRatio,      // aspectRatio
		0.1,              // aperture
		10,               // focusDist
	)

	// Build the scene
	world := &HittableList{}

	// Ground: large sphere
	world.Add(Sphere{
		Center: Vec3{0, -1000, 0},
		Radius: 1000,
		Mat:    Lambertian{Vec3{0.5, 0.5, 0.5}},
	})

	// Center glass sphere
	world.Add(Sphere{
		Center: Vec3{0, 1, 0},
		Radius: 1,
		Mat:    Dielectric{1.5},
	})

	// Left diffuse sphere
	world.Add(Sphere{
		Center: Vec3{-4, 1, 0},
		Radius: 1,
		Mat:    Lambertian{Vec3{0.4, 0.2, 0.1}},
	})

	// Right metal sphere
	world.Add(Sphere{
		Center: Vec3{4, 1, 0},
		Radius: 1,
		Mat:    Metal{Vec3{0.7, 0.6, 0.5}, 0.0},
	})

	// Random small spheres
	for i := -11; i < 11; i++ {
		for j := -11; j < 11; j++ {
			chooseMat := rand.Float64()
			center := Vec3{
				float64(i) + 0.9*rand.Float64(),
				0.2,
				float64(j) + 0.9*rand.Float64(),
			}

			// Skip if too close to the main spheres
			if center.Sub(Vec3{4, 0.2, 0}).Length() > 0.9 {
				var material Material

				if chooseMat < 0.8 {
					// Diffuse (Lambertian)
					albedo := RandomVec3().MulVec(RandomVec3())
					material = Lambertian{albedo}
				} else if chooseMat < 0.95 {
					// Metal
					albedo := RandomVec3InRange(0.5, 1)
					fuzz := rand.Float64() * 0.5
					material = Metal{albedo, fuzz}
				} else {
					// Glass (Dielectric)
					material = Dielectric{1.5}
				}

				world.Add(Sphere{
					Center: center,
					Radius: 0.2,
					Mat:    material,
				})
			}
		}
	}

	// Output PPM header
	fmt.Printf("P3\n%d %d\n255\n", imageWidth, imageHeight)

	// Render the image
	for j := imageHeight - 1; j >= 0; j-- {
		// Print progress to stderr
		fmt.Fprintf(os.Stderr, "\rScanlines remaining: %d ", j)

		for i := 0; i < imageWidth; i++ {
			pixelColor := Vec3{0, 0, 0}

			// Accumulate samples
			for s := 0; s < samplesPerPixel; s++ {
				// Jitter the sample within the pixel
				u := (float64(i) + rand.Float64()) / float64(imageWidth-1)
				v := (float64(j) + rand.Float64()) / float64(imageHeight-1)

				// Get ray from camera and trace it
				r := camera.GetRay(u, v)
				pixelColor = pixelColor.Add(rayColor(r, world, maxDepth))
			}

			// Average the samples and apply gamma correction (gamma = 2.0, so sqrt)
			scale := 1.0 / float64(samplesPerPixel)
			pixelColor = pixelColor.Scale(scale)

			// Gamma correct: color^(1/2)
			r := math.Sqrt(pixelColor.X)
			g := math.Sqrt(pixelColor.Y)
			b := math.Sqrt(pixelColor.Z)

			// Clamp to [0, 255]
			rInt := int(255.999 * r)
			gInt := int(255.999 * g)
			bInt := int(255.999 * b)

			if rInt > 255 {
				rInt = 255
			}
			if gInt > 255 {
				gInt = 255
			}
			if bInt > 255 {
				bInt = 255
			}

			fmt.Printf("%d %d %d\n", rInt, gInt, bInt)
		}
	}

	fmt.Fprintf(os.Stderr, "\nDone.\n")
}
