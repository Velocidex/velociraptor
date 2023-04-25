import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import api from '../core/api-service.jsx';
import {CancelToken} from 'axios';
import Modal from 'react-bootstrap/Modal';
import Button from 'react-bootstrap/Button';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Navbar from 'react-bootstrap/Navbar';
import VeloAce, { SettingsButton } from '../core/ace.jsx';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import Completer from '../artifacts/syntax.jsx';

const artifact_template = `name: Custom.MyNewArtifact
description: |
   This is the human readable description of the artifact.

# Can be CLIENT, CLIENT_EVENT, SERVER, SERVER_EVENT
type: CLIENT

# Add tools by defining here and consume by
# LET bin <= SELECT * FROM Artifact.Utils.FetchBinary(name="MyToolName")
#tools:
# - name: MyToolName
#   url: http://www.example.com

parameters:
   - name: FirstParameter
     default: |
        Default Value of first parameter

precondition:
      SELECT OS From info() where OS = 'windows'
        OR OS = 'linux'
        OR OS = 'darwin'

sources:
  - query: |
`;

export default class CreateArtifactFromCell extends React.Component {
    static propTypes = {
        vql: PropTypes.string,
        onClose: PropTypes.func.isRequired,
    };

    // Create an artifact template from the VQL
    componentDidMount = () => {
        this.source = CancelToken.source();
        let lines = _.map(this.props.vql.match(/[^\r\n]+/g), line=>"      "+line);
        this.setState({text: artifact_template + lines.join("\n")});
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    state = {
        text: "",
        loading: false,
        ace: null,
    }

    saveArtifact = () => {
        api.post("v1/SetArtifactFile", {artifact: this.state.text},
                this.source.token).then(
            (response) => {
                this.props.onClose();
            });
    }

    aceConfig = (ace) => {
        // Attach a completer to ACE.
        let completer = new Completer();
        completer.initializeAceEditor(ace, {});

        ace.setOptions({
            wrap: true,
            autoScrollEditorIntoView: true,
            minLines: 10,
            maxLines: 1000,
        });

        ace.resize();

        // Hold a reference to the ace editor.
        this.setState({ace: ace});
    };

    render() {
        return (
            <Modal show={true}
                   className="full-height"
                   dialogClassName="modal-90w"
                   enforceFocus={false}
                   scrollable={true}
                   onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>Create a new artifact from VQL cell</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <VeloAce text={this.state.text}
                         mode="yaml"
                         aceConfig={this.aceConfig}
                         onChange={(x) => this.setState({text: x})}
                         commands={[{
                             name: 'saveAndExit',
                             bindKey: {win: 'Ctrl-Enter',  mac: 'Command-Enter'},
                             exec: (editor) => {
                                 this.saveArtifact();
                             },
                         }]}
                />
              </Modal.Body>
              <Modal.Footer>
                <Navbar className="w-100 justify-content-between">
                <ButtonGroup className="float-left">
                  <SettingsButton ace={this.state.ace}/>
                </ButtonGroup>

                <ButtonGroup className="float-right">
                  <Button variant="default"
                          onClick={this.props.onClose}>
                    <FontAwesomeIcon icon="window-close"/>
                    <span className="button-label">Close</span>
                  </Button>
                  <Button variant="primary"
                          onClick={this.saveArtifact}>
                    <FontAwesomeIcon icon="save"/>
                    <span className="button-label">Save</span>
                  </Button>
                </ButtonGroup>
                </Navbar>
              </Modal.Footer>
            </Modal>
        );
    }
};
