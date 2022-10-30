package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/pkg/errors"
)

// ZKInputs are inputs for proof generation
type ZKInputs map[string]interface{}

// ZKProof is structure that represents SnarkJS library result of proof generation
type ZKProof struct {
	A        []string   `json:"pi_a"`
	B        [][]string `json:"pi_b"`
	C        []string   `json:"pi_c"`
	Protocol string     `json:"protocol"`
	Curve    string     `json:"curve"`
}

// FullProof is ZKP proof with public signals
type FullProof struct {
	Proof      *ZKProof `json:"proof"`
	PubSignals []string `json:"pub_signals"`
}

// GenerateZkProof executes snarkjs groth16prove function and returns proof only if it's valid
func GenerateZkProof(circuitPath string, inputs ZKInputs) (*FullProof, error) {

	if path.Clean(circuitPath) != circuitPath {
		return nil, fmt.Errorf("illegal circuitPath")
	}

	// serialize inputs into json
	inputsJSON, err := json.Marshal(inputs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to serialize inputs into json")
	}

	// create tmf file for inputs
	inputFile, err := ioutil.TempFile("tmp", "input-*.json")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create tmf file for inputs")
	}
	defer os.Remove(inputFile.Name())

	// write json inputs into tmp file
	_, err = inputFile.Write(inputsJSON)
	if err != nil {
		return nil, errors.Wrap(err, "failed to write json inputs into tmp file")
	}
	err = inputFile.Close()
	if err != nil {
		return nil, errors.Wrap(err, "failed to close json inputs tmp file")
	}

	// create tmp witness file
	wtnsFile, err := ioutil.TempFile("tmp", "witness-*.wtns")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create tmp witness file")
	}
	defer os.Remove(wtnsFile.Name())
	err = wtnsFile.Close()
	if err != nil {
		return nil, errors.Wrap(err, "failed to close tmp witness file")
	}

	// calculate witness
	wtnsCmd := exec.Command("node", "js/generate_witness.js", circuitPath+"/circuit.wasm", inputFile.Name(), wtnsFile.Name())
	wtnsOut, err := wtnsCmd.CombinedOutput()
	if err != nil {
		log.Println("failed to calculate witness", "wtnsOut", string(wtnsOut))
		return nil, errors.Wrap(err, "failed to calculate witness")
	}
	log.Println("-- witness calculate completed --")

	// create tmp proof file
	proofFile, err := ioutil.TempFile("tmp", "proof-*.json")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create tmp proof file")
	}
	defer os.Remove(proofFile.Name())
	err = proofFile.Close()
	if err != nil {
		return nil, errors.Wrap(err, "failed to close tmp proof file")
	}

	// create tmp public file
	publicFile, err := ioutil.TempFile("tmp", "public-*.json")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create tmp public file")
	}
	defer os.Remove(publicFile.Name())
	err = publicFile.Close()
	if err != nil {
		return nil, errors.Wrap(err, "failed to close tmp public file")
	}

	// generate proof
	var execCommandName string
	var execCommandParams []string
	execCommandName = "snarkjs"
	execCommandParams = append(execCommandParams, "groth16", "prove")
	execCommandParams = append(execCommandParams, circuitPath+"/circuit_final.zkey", wtnsFile.Name(), proofFile.Name(), publicFile.Name())
	proveCmd := exec.Command(execCommandName, execCommandParams...)
	log.Println("used prover: %s", execCommandName)
	proveOut, err := proveCmd.CombinedOutput()
	if err != nil {
		log.Println("failed to generate proof", "proveOut", string(proveOut))
		return nil, errors.Wrap(err, "failed to generate proof")
	}
	log.Println("-- groth16 prove completed --")

	// verify proof
	verifyCmd := exec.Command("snarkjs", "groth16", "verify", circuitPath+"/verification_key.json", publicFile.Name(), proofFile.Name())
	verifyOut, err := verifyCmd.CombinedOutput()
	if err != nil {
		return nil, errors.Wrap(err, "failed to verify proof")
	}
	log.Println("-- groth16 verify -- snarkjs result %s", strings.TrimSpace(string(verifyOut)))

	if !strings.Contains(string(verifyOut), "OK!") {
		return nil, errors.New("invalid proof")
	}

	var proof ZKProof
	var pubSignals []string

	// read generated public signals
	publicJSON, err := os.ReadFile(publicFile.Name())
	if err != nil {
		return nil, errors.Wrap(err, "failed to read generated public signals")
	}

	err = json.Unmarshal(publicJSON, &pubSignals)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal public signals")
	}
	// read generated proof
	proofJSON, err := os.ReadFile(proofFile.Name())
	if err != nil {
		return nil, errors.Wrap(err, "failed to read generated proof")
	}

	err = json.Unmarshal(proofJSON, &proof)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal generated proof")
	}

	return &FullProof{Proof: &proof, PubSignals: pubSignals}, nil
}

// VerifyZkProof executes snarkjs verify function and returns if proof is valid
func VerifyZkProof(circuitPath string, zkp *FullProof) error {

	if path.Clean(circuitPath) != circuitPath {
		return fmt.Errorf("illegal circuitPath")
	}

	// create tmp proof file
	proofFile, err := ioutil.TempFile("tmp", "proof-*.json")
	if err != nil {
		return errors.Wrap(err, "failed to create tmp proof file")
	}
	defer os.Remove(proofFile.Name())

	// create tmp public file
	publicFile, err := ioutil.TempFile("tmp", "public-*.json")
	if err != nil {
		return errors.Wrap(err, "failed to create tmp public file")
	}
	defer os.Remove(publicFile.Name())

	// serialize proof into json
	proofJSON, err := json.Marshal(zkp.Proof)
	if err != nil {
		return errors.Wrap(err, "failed to serialize proof into json")
	}

	// serialize public signals into json
	publicJSON, err := json.Marshal(zkp.PubSignals)
	if err != nil {
		return errors.Wrap(err, "failed to serialize public signals into json")
	}

	// write json proof into tmp file
	_, err = proofFile.Write(proofJSON)
	if err != nil {
		return errors.Wrap(err, "failed to write json proof into tmp file")
	}
	err = proofFile.Close()
	if err != nil {
		return errors.Wrap(err, "failed to close json proof tmp file")
	}

	// write json public signals into tmp file
	_, err = publicFile.Write(publicJSON)
	if err != nil {
		return errors.Wrap(err, "failed to write json public signals into tmp file")
	}
	err = publicFile.Close()
	if err != nil {
		return errors.Wrap(err, "failed to close json public signals tmp file")
	}

	// verify proof
	verifyCmd := exec.Command("snarkjs", "groth16", "verify", circuitPath+"/verification_key.json", publicFile.Name(), proofFile.Name())
	verifyOut, err := verifyCmd.CombinedOutput()
	if err != nil {
		return errors.Wrap(err, "failed to verify proof")
	}
	log.Println("-- groth16 verify -- snarkjs result %s", strings.TrimSpace(string(verifyOut)))

	if !strings.Contains(string(verifyOut), "OK!") {
		return errors.New("invalid proof")
	}

	return nil
}
