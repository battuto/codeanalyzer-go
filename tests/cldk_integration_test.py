#!/usr/bin/env python3
"""
CLDK Integration Tests for codeanalyzer-go

Tests the Go analyzer output against CLDK schema expectations.
Run with: python tests/cldk_integration_test.py

Prerequisites:
- Build the analyzer: .\\scripts\\build.ps1
- Have test targets: sampleapp/ and frp_goleash_campaign/target/
"""

import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

# Determine paths
SCRIPT_DIR = Path(__file__).parent.absolute()
PROJECT_ROOT = SCRIPT_DIR.parent
ANALYZER_PATH = PROJECT_ROOT / "bin" / "codeanalyzer-go-windows-amd64.exe"
SAMPLE_APP = PROJECT_ROOT / "sampleapp"
FRP_TARGET = PROJECT_ROOT / "frp_goleash_campaign" / "target"


def run_analyzer(*args: str, capture_output: bool = True) -> subprocess.CompletedProcess:
    """Run the analyzer with given arguments."""
    cmd = [str(ANALYZER_PATH)] + list(args)
    return subprocess.run(
        cmd,
        capture_output=capture_output,
        text=True,
        cwd=str(PROJECT_ROOT),
    )


# ============================================================================
# Core schema tests (sampleapp)
# ============================================================================

class TestCLDKSchema(unittest.TestCase):
    """Test that output conforms to CLDK schema."""

    def test_version_flag(self):
        """Test --version flag returns version string."""
        result = run_analyzer("--version")
        self.assertEqual(result.returncode, 0)
        self.assertIn("codeanalyzer-go", result.stdout)

    def test_symbol_table_output_structure(self):
        """Test symbol_table analysis level produces valid structure."""
        result = run_analyzer(
            "--input", str(SAMPLE_APP),
            "--analysis-level", "symbol_table",
        )
        self.assertEqual(result.returncode, 0, f"stderr: {result.stderr}")

        data = json.loads(result.stdout)

        # Verify metadata
        self.assertIn("metadata", data)
        metadata = data["metadata"]
        self.assertEqual(metadata["analyzer"], "codeanalyzer-go")
        self.assertEqual(metadata["language"], "go")
        self.assertEqual(metadata["analysis_level"], "symbol_table")
        self.assertIn("version", metadata)
        self.assertIn("timestamp", metadata)
        self.assertIn("go_version", metadata)
        self.assertIn("analysis_duration_ms", metadata)

        # Verify symbol_table
        self.assertIn("symbol_table", data)
        st = data["symbol_table"]
        self.assertIn("packages", st)
        self.assertIsInstance(st["packages"], dict)

        # Verify at least one package exists
        self.assertGreater(len(st["packages"]), 0)

        # Verify package structure
        for pkg_path, pkg in st["packages"].items():
            self.assertIn("path", pkg)
            self.assertIn("name", pkg)
            self.assertIn("files", pkg)
            self.assertIn("imports", pkg)
            self.assertIn("type_declarations", pkg)
            self.assertIn("callable_declarations", pkg)
            self.assertIn("variables", pkg)
            self.assertIn("constants", pkg)

            # Verify maps are dicts
            self.assertIsInstance(pkg["type_declarations"], dict)
            self.assertIsInstance(pkg["callable_declarations"], dict)
            self.assertIsInstance(pkg["variables"], dict)
            self.assertIsInstance(pkg["constants"], dict)

        # Verify issues array
        self.assertIn("issues", data)
        self.assertIsInstance(data["issues"], list)

        # Verify PDG/SDG are null placeholders
        self.assertIn("pdg", data)
        self.assertIn("sdg", data)

    def test_call_graph_output_structure(self):
        """Test call_graph analysis level produces valid structure."""
        result = run_analyzer(
            "--input", str(SAMPLE_APP),
            "--analysis-level", "call_graph",
            "--cg", "cha",
        )
        self.assertEqual(result.returncode, 0, f"stderr: {result.stderr}")

        data = json.loads(result.stdout)

        # Verify call_graph
        self.assertIn("call_graph", data)
        cg = data["call_graph"]
        self.assertIn("algorithm", cg)
        self.assertIn("nodes", cg)
        self.assertIn("edges", cg)
        self.assertIsInstance(cg["nodes"], list)
        self.assertIsInstance(cg["edges"], list)

        # Verify node structure
        for node in cg["nodes"]:
            self.assertIn("id", node)
            self.assertIn("qualified_name", node)
            self.assertIn("name", node)
            self.assertIn("kind", node)
            self.assertIn(node["kind"], ["function", "method"])

        # Verify edge structure
        for edge in cg["edges"]:
            self.assertIn("source", edge)
            self.assertIn("target", edge)

    def test_full_analysis_output(self):
        """Test full analysis level includes both symbol_table and call_graph."""
        result = run_analyzer(
            "--input", str(SAMPLE_APP),
            "--analysis-level", "full",
            "--cg", "rta",
        )
        self.assertEqual(result.returncode, 0, f"stderr: {result.stderr}")

        data = json.loads(result.stdout)

        # Both should be present
        self.assertIn("symbol_table", data)
        self.assertIn("call_graph", data)
        self.assertIsNotNone(data["symbol_table"])
        # call_graph might be None if no main packages found

    def test_file_output(self):
        """Test output to file via --output flag."""
        with tempfile.TemporaryDirectory() as tmpdir:
            result = run_analyzer(
                "--input", str(SAMPLE_APP),
                "--analysis-level", "symbol_table",
                "--output", tmpdir,
            )
            self.assertEqual(result.returncode, 0, f"stderr: {result.stderr}")

            # Verify file was created
            output_file = Path(tmpdir) / "analysis.json"
            self.assertTrue(output_file.exists())

            # Verify content is valid JSON
            with open(output_file) as f:
                data = json.load(f)
            self.assertIn("metadata", data)

    def test_callable_declarations_structure(self):
        """Test callable_declarations have correct CLDK structure."""
        result = run_analyzer(
            "--input", str(SAMPLE_APP),
            "--analysis-level", "symbol_table",
        )
        self.assertEqual(result.returncode, 0)

        data = json.loads(result.stdout)

        for pkg_path, pkg in data["symbol_table"]["packages"].items():
            for qn, callable_decl in pkg["callable_declarations"].items():
                self.assertIn("qualified_name", callable_decl)
                self.assertIn("name", callable_decl)
                self.assertIn("signature", callable_decl)
                self.assertIn("kind", callable_decl)
                self.assertIn("parameters", callable_decl)
                self.assertIn("results", callable_decl)
                self.assertIn("exported", callable_decl)

                # Verify kind is function or method
                self.assertIn(callable_decl["kind"], ["function", "method"])

                # Verify parameters structure (can be None or empty list)
                params = callable_decl["parameters"]
                if params:
                    for param in params:
                        self.assertIn("type", param)

                # If method, should have receiver info
                if callable_decl["kind"] == "method":
                    self.assertIn("receiver_type", callable_decl)

    def test_type_declarations_structure(self):
        """Test type_declarations have correct CLDK structure."""
        result = run_analyzer(
            "--input", str(SAMPLE_APP),
            "--analysis-level", "symbol_table",
        )
        self.assertEqual(result.returncode, 0)

        data = json.loads(result.stdout)

        for pkg_path, pkg in data["symbol_table"]["packages"].items():
            for qn, type_decl in pkg["type_declarations"].items():
                self.assertIn("qualified_name", type_decl)
                self.assertIn("name", type_decl)
                self.assertIn("kind", type_decl)

                # Verify kind is valid
                self.assertIn(type_decl["kind"], ["struct", "interface", "alias", "named"])

                # Structs should have fields
                if type_decl["kind"] == "struct" and "fields" in type_decl:
                    for field in type_decl["fields"]:
                        self.assertIn("name", field)
                        self.assertIn("type", field)
                        self.assertIn("exported", field)


# ============================================================================
# New features tests (sampleapp)
# ============================================================================

class TestCLDKNewFeatures(unittest.TestCase):
    """Test new features: interface methods, package doc, call examples, docstrings."""

    @classmethod
    def setUpClass(cls):
        """Run analyzer once with --include-body for all tests."""
        result = run_analyzer(
            "--input", str(SAMPLE_APP),
            "--analysis-level", "symbol_table",
            "--include-body",
        )
        assert result.returncode == 0, f"Analyzer failed: {result.stderr}"
        cls.data = json.loads(result.stdout)
        cls.pkg = list(cls.data["symbol_table"]["packages"].values())[0]

    def test_package_documentation(self):
        """Test package-level documentation is extracted."""
        doc = self.pkg.get("documentation", "")
        self.assertIn("Package main", doc)
        self.assertTrue(len(doc) > 10, "Package documentation should be substantive")

    def test_interface_methods_greeter(self):
        """Test Greeter interface has interface_methods extracted."""
        greeter = None
        for qn, td in self.pkg["type_declarations"].items():
            if td["name"] == "Greeter":
                greeter = td
                break
        self.assertIsNotNone(greeter, "Greeter interface not found")
        self.assertEqual(greeter["kind"], "interface")

        ims = greeter.get("interface_methods", [])
        self.assertGreater(len(ims), 0, "Greeter should have interface_methods")

        # Verify Greet method
        greet = ims[0]
        self.assertEqual(greet["name"], "Greet")
        self.assertIn("signature", greet)
        self.assertIn("parameters", greet)
        self.assertIn("results", greet)

    def test_interface_methods_calculator(self):
        """Test Calculator interface has documented methods."""
        calc = None
        for qn, td in self.pkg["type_declarations"].items():
            if td["name"] == "Calculator":
                calc = td
                break
        self.assertIsNotNone(calc, "Calculator interface not found")

        ims = calc.get("interface_methods", [])
        self.assertEqual(len(ims), 2, "Calculator should have 2 interface_methods (Add, Multiply)")

        names = [m["name"] for m in ims]
        self.assertIn("Add", names)
        self.assertIn("Multiply", names)

        # Check documentation on interface methods
        for m in ims:
            self.assertTrue(
                m.get("documentation", "") != "",
                f"Interface method {m['name']} should have documentation"
            )

    def test_interface_method_structure(self):
        """Test CLDKInterfaceMethod has all required fields."""
        for qn, td in self.pkg["type_declarations"].items():
            ims = td.get("interface_methods", [])
            for im in ims:
                self.assertIn("name", im)
                self.assertIn("signature", im)
                self.assertIn("parameters", im)
                self.assertIn("results", im)
                # documentation is optional

    def test_function_docstrings(self):
        """Test function documentation is extracted."""
        # 'add' has a docstring: "add returns the sum of two ints."
        for qn, cd in self.pkg["callable_declarations"].items():
            if cd["name"] == "add":
                self.assertIn("documentation", cd)
                self.assertIn("sum", cd["documentation"].lower())
                return
        self.fail("Function 'add' not found")

    def test_call_examples_with_body(self):
        """Test call_examples are populated when --include-body is used."""
        has_examples = False
        for qn, cd in self.pkg["callable_declarations"].items():
            exs = cd.get("call_examples", [])
            if exs:
                has_examples = True
                for ex in exs:
                    self.assertIsInstance(ex, str)
                    self.assertIn("called by", ex)
        self.assertTrue(has_examples, "At least one callable should have call_examples")

    def test_call_examples_without_body(self):
        """Test call_examples are NOT present without --include-body."""
        result = run_analyzer(
            "--input", str(SAMPLE_APP),
            "--analysis-level", "symbol_table",
        )
        self.assertEqual(result.returncode, 0)
        data = json.loads(result.stdout)
        pkg = list(data["symbol_table"]["packages"].values())[0]

        for qn, cd in pkg["callable_declarations"].items():
            self.assertFalse(
                cd.get("call_examples"),
                f"call_examples should not be present without --include-body for {cd['name']}"
            )


# ============================================================================
# Compact output tests (sampleapp)
# ============================================================================

class TestCLDKCompact(unittest.TestCase):
    """Test compact output format for LLM compatibility."""

    @classmethod
    def setUpClass(cls):
        """Run analyzer once in compact mode with --include-body."""
        result = run_analyzer(
            "--input", str(SAMPLE_APP),
            "--analysis-level", "symbol_table",
            "--include-body",
            "--compact",
        )
        assert result.returncode == 0, f"Analyzer failed: {result.stderr}"
        cls.data = json.loads(result.stdout)
        cls.pkg = list(cls.data["p"].values())[0]

    def test_compact_meta(self):
        """Test compact metadata structure."""
        m = self.data["m"]
        self.assertIn("v", m)  # version
        self.assertIn("l", m)  # language
        self.assertIn("a", m)  # analysis level
        self.assertIn("d", m)  # duration

    def test_compact_package_doc(self):
        """Test compact package documentation (d field)."""
        doc = self.pkg.get("d", "")
        self.assertIn("Package main", doc)

    def test_compact_interface_methods(self):
        """Test compact interface methods (im field)."""
        types = self.pkg.get("t", {})
        found_im = False
        for name, t in types.items():
            ims = t.get("im", [])
            if ims:
                found_im = True
                for sig in ims:
                    self.assertIsInstance(sig, str)
        self.assertTrue(found_im, "At least one type should have im (interface methods)")

    def test_compact_call_examples(self):
        """Test compact call examples (ex field)."""
        funcs = self.pkg.get("fn", {})
        found_ex = False
        for name, f in funcs.items():
            exs = f.get("ex", [])
            if exs:
                found_ex = True
                for ex in exs:
                    self.assertIsInstance(ex, str)
        self.assertTrue(found_ex, "At least one function should have ex (call examples)")

    def test_compact_func_structure(self):
        """Test compact function structure has required fields."""
        funcs = self.pkg.get("fn", {})
        self.assertGreater(len(funcs), 0)
        for name, f in funcs.items():
            self.assertIn("s", f)  # signature is required

    def test_compact_type_structure(self):
        """Test compact type structure has required fields."""
        types = self.pkg.get("t", {})
        self.assertGreater(len(types), 0)
        for name, t in types.items():
            self.assertIn("k", t)  # kind is required


# ============================================================================
# FRP target tests (real-world Go project)
# ============================================================================

class TestFRPTarget(unittest.TestCase):
    """Test analyzer on a real-world Go project (frp)."""

    @classmethod
    def setUpClass(cls):
        """Run analyzer on frp target."""
        result = run_analyzer(
            "--input", str(FRP_TARGET),
            "--analysis-level", "symbol_table",
            "--include-body",
        )
        assert result.returncode == 0, f"Analyzer failed: {result.stderr}"
        cls.data = json.loads(result.stdout)

    def test_frp_multiple_packages(self):
        """Test that FRP project produces multiple packages."""
        pkgs = self.data["symbol_table"]["packages"]
        self.assertGreater(len(pkgs), 5, "FRP should have many packages")

    def test_frp_has_interfaces(self):
        """Test that FRP project has interfaces with interface_methods."""
        found_interface = False
        for pkg_path, pkg in self.data["symbol_table"]["packages"].items():
            for qn, td in pkg["type_declarations"].items():
                if td["kind"] == "interface":
                    found_interface = True
                    ims = td.get("interface_methods", [])
                    if ims:
                        # Verify structure
                        for im in ims:
                            self.assertIn("name", im)
                            self.assertIn("signature", im)
                            self.assertIn("parameters", im)
                            self.assertIn("results", im)
                        return  # Found at least one interface with methods
        self.assertTrue(found_interface, "FRP should have at least one interface")

    def test_frp_has_documentation(self):
        """Test that FRP project package documentation field exists (may be empty)."""
        # Real-world projects may not always have package doc comments
        for pkg_path, pkg in self.data["symbol_table"]["packages"].items():
            # Just verify the field can be accessed without error
            doc = pkg.get("documentation", "")
            self.assertIsInstance(doc, str)

    def test_frp_has_structs_with_methods(self):
        """Test that FRP project has structs with methods."""
        for pkg_path, pkg in self.data["symbol_table"]["packages"].items():
            for qn, td in pkg["type_declarations"].items():
                if td["kind"] == "struct" and td.get("methods"):
                    self.assertIsInstance(td["methods"], dict)
                    for mq, m in td["methods"].items():
                        self.assertIn("name", m)
                        self.assertIn("signature", m)
                    return
        self.fail("FRP should have structs with methods")

    def test_frp_has_callable_declarations(self):
        """Test that FRP project has callable declarations with full structure."""
        total_callables = 0
        for pkg_path, pkg in self.data["symbol_table"]["packages"].items():
            for qn, cd in pkg["callable_declarations"].items():
                total_callables += 1
                self.assertIn("qualified_name", cd)
                self.assertIn("name", cd)
                self.assertIn("signature", cd)
                self.assertIn("kind", cd)
                self.assertIn(cd["kind"], ["function", "method"])
        self.assertGreater(total_callables, 20, "FRP should have many callable declarations")

    def test_frp_has_call_examples(self):
        """Test that FRP functions have call_examples."""
        found_examples = False
        for pkg_path, pkg in self.data["symbol_table"]["packages"].items():
            for qn, cd in pkg["callable_declarations"].items():
                if cd.get("call_examples"):
                    found_examples = True
                    break
            if found_examples:
                break
        self.assertTrue(found_examples, "FRP should have functions with call_examples")

    def test_frp_compact_output(self):
        """Test compact output on FRP project."""
        result = run_analyzer(
            "--input", str(FRP_TARGET),
            "--analysis-level", "symbol_table",
            "--compact",
        )
        self.assertEqual(result.returncode, 0, f"stderr: {result.stderr}")

        data = json.loads(result.stdout)

        # Verify compact structure
        self.assertIn("m", data)    # meta
        self.assertIn("p", data)    # packages
        self.assertGreater(len(data["p"]), 0)

        # Doc may or may not be present in real projects
        has_doc = any(p.get("d") for p in data["p"].values())
        # Just verify structure is correct, don't require doc
        self.assertGreater(len(data["p"]), 0)

    def test_frp_full_analysis(self):
        """Test full analysis (symbol_table + call_graph) on FRP."""
        result = run_analyzer(
            "--input", str(FRP_TARGET),
            "--analysis-level", "full",
            "--cg", "rta",
        )
        self.assertEqual(result.returncode, 0, f"stderr: {result.stderr}")

        data = json.loads(result.stdout)
        self.assertIn("symbol_table", data)
        self.assertIn("call_graph", data)
        self.assertIsNotNone(data["symbol_table"])


# ============================================================================
# Legacy compatibility tests
# ============================================================================

class TestCLDKLegacyCompatibility(unittest.TestCase):
    """Test backward compatibility with legacy flags."""

    def test_legacy_root_flag(self):
        """Test --root flag still works with deprecation warning."""
        result = run_analyzer(
            "--root", str(SAMPLE_APP),
            "--analysis-level", "symbol_table",
        )
        self.assertEqual(result.returncode, 0)

        # Should have deprecation warning on stderr
        self.assertIn("deprecated", result.stderr.lower())

        # Output should still be valid
        data = json.loads(result.stdout)
        self.assertIn("metadata", data)

    def test_legacy_mode_flag(self):
        """Test --mode flag still works with deprecation warning."""
        result = run_analyzer(
            "--input", str(SAMPLE_APP),
            "--mode", "symbol-table",
        )
        self.assertEqual(result.returncode, 0)

        # Should have deprecation warning on stderr
        self.assertIn("deprecated", result.stderr.lower())


# ============================================================================
# Position tests
# ============================================================================

class TestCLDKPositions(unittest.TestCase):
    """Test position information in output."""

    def test_detailed_positions(self):
        """Test emit-positions=detailed includes position info."""
        result = run_analyzer(
            "--input", str(SAMPLE_APP),
            "--analysis-level", "symbol_table",
            "--emit-positions", "detailed",
        )
        self.assertEqual(result.returncode, 0)

        data = json.loads(result.stdout)

        # Check that positions are included
        for pkg_path, pkg in data["symbol_table"]["packages"].items():
            for qn, callable_decl in pkg["callable_declarations"].items():
                if "position" in callable_decl and callable_decl["position"]:
                    pos = callable_decl["position"]
                    self.assertIn("file", pos)
                    self.assertIn("start_line", pos)
                    self.assertIn("start_column", pos)
                    break  # Found at least one with position

    def test_minimal_positions(self):
        """Test emit-positions=minimal omits position info."""
        result = run_analyzer(
            "--input", str(SAMPLE_APP),
            "--analysis-level", "symbol_table",
            "--emit-positions", "minimal",
        )
        self.assertEqual(result.returncode, 0)

        data = json.loads(result.stdout)

        # Positions should be null/missing
        for pkg_path, pkg in data["symbol_table"]["packages"].items():
            for qn, callable_decl in pkg["callable_declarations"].items():
                pos = callable_decl.get("position")
                self.assertIsNone(pos)


# ============================================================================
# Error handling tests
# ============================================================================

class TestCLDKErrorHandling(unittest.TestCase):
    """Test error handling and exit codes."""

    def test_invalid_input_path(self):
        """Test exit code 2 for invalid input path."""
        result = run_analyzer(
            "--input", "/nonexistent/path/to/project",
            "--analysis-level", "symbol_table",
        )
        self.assertEqual(result.returncode, 2)
        self.assertIn("error", result.stderr.lower())

    def test_invalid_analysis_level(self):
        """Test exit code 2 for invalid analysis level."""
        result = run_analyzer(
            "--input", str(SAMPLE_APP),
            "--analysis-level", "invalid_level",
        )
        self.assertEqual(result.returncode, 2)

    def test_invalid_cg_algorithm(self):
        """Test exit code 2 for invalid call graph algorithm."""
        result = run_analyzer(
            "--input", str(SAMPLE_APP),
            "--analysis-level", "call_graph",
            "--cg", "invalid_algo",
        )
        self.assertEqual(result.returncode, 2)


if __name__ == "__main__":
    # Check if analyzer exists before running tests
    if not ANALYZER_PATH.exists():
        print(f"ERROR: Analyzer not found at {ANALYZER_PATH}")
        print("Run: .\\scripts\\build.ps1")
        sys.exit(1)
    unittest.main(verbosity=2)
