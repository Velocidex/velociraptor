import React from 'react';
import PropTypes from 'prop-types';

import Modal from 'react-bootstrap/Modal';
import Button from 'react-bootstrap/Button';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Navbar from 'react-bootstrap/Navbar';
import VeloAce, { SettingsButton } from '../core/ace.js';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

import api from '../core/api-service.js';
import axios from 'axios';
import Completer from './syntax.js';

export default class NewArtifactDialog extends React.Component {
    static propTypes = {
        name: PropTypes.string,
        onClose: PropTypes.func.isRequired,
    };

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
        this.fetchArtifact();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        this.fetchArtifact();
    }

    state = {
        text: "",
        initialized_from_parent: false,
        loading: false,
        ace: null,
    }

    fetchArtifact = () => {
        if (!this.state.initialized_from_parent) {
            this.setState({loading: true, initialized_from_parent: true});

            api.get("v1/GetArtifactFile",
                    {name: this.props.name}, this.source.token).then(response=>{
                    this.setState({
                        text: response.data.artifact,
                        loading: false,
                    });

                });
        }
    }

    saveArtifact = () => {
        api.post("v1/SetArtifactFile",
                 {artifact: this.state.text}, this.source.token).then(
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
                <Modal.Title>{ this.props.name ?
                               "Edit " + this.props.name :
                               "Create a new artifact"}
                </Modal.Title>
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
