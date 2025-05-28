import _ from 'lodash';

import React from 'react';
import PropTypes from 'prop-types';

import Alert from 'react-bootstrap/Alert';
import Modal from 'react-bootstrap/Modal';
import Button from 'react-bootstrap/Button';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Navbar from 'react-bootstrap/Navbar';
import VeloAce, { SettingsButton } from '../core/ace.jsx';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import T from '../i8n/i8n.jsx';
import api from '../core/api-service.jsx';
import {CancelToken} from 'axios';
import Completer from './syntax.jsx';
import VeloLog from '../widgets/logs.jsx';

// A dialog showing any errors with the artifact.
class SetArtifactErrorDialog extends React.Component {
    static propTypes = {
        error: PropTypes.object,
        // Dismiss the error so the artifact can be fixed
        onClose: PropTypes.func.isRequired,

        // Ignore the error and set it anyway.
        onIgnore: PropTypes.func.isRequired,
    };

    render() {
        return (
            <Modal show={true}
                   className="full-height"
                   dialogClassName="modal-90w"
                   enforceFocus={false}
                   keyboard={false}
                   scrollable={true}>
              <Modal.Header closeButton
                            onHide={this.props.onClose}>
                <Modal.Title>{ T("Problems detected with artifact") }</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                {_.map(this.props.error.errors, (x, idx)=>{
                    return <Alert variant="danger" key={idx}>
                             <VeloLog value={x}/>
                           </Alert>;
                })}

                {_.map(this.props.error.warnings, (x, idx)=>{
                    return <Alert variant="warning" key={idx}>
                             <VeloLog value={x}/>
                           </Alert>;
                })}

              </Modal.Body>
              <Modal.Footer>
                <Navbar className="w-100 justify-content-between">
                  <ButtonGroup className="float-right">
                    <Button variant="default"
                            onClick={this.props.onClose}>
                      <FontAwesomeIcon icon="window-close"/>
                      <span className="button-label">{T("Fix Artifact")}</span>
                    </Button>
                    <Button variant="primary"
                            onClick={this.props.onIgnore}>
                      <FontAwesomeIcon icon="save"/>
                      <span className="button-label">{T("Save Anyway")}</span>
                    </Button>
                  </ButtonGroup>
                </Navbar>
              </Modal.Footer>
            </Modal>
        );
    }
}

export default class NewArtifactDialog extends React.Component {
    static propTypes = {
        name: PropTypes.string,
        onClose: PropTypes.func.isRequired,
    };

    componentDidMount = () => {
        this.source = CancelToken.source();
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
        error_state: {},
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

    checkArtifact = () => {
        api.post("v1/SetArtifactFile", {
            artifact: this.state.text,
            op: "CHECK_AND_SET",
        }, this.source.token).then(response=> {
                if (response.cancel) {
                    return;
                }
                let state = response.data || {};

                // No problems detected!
                if (_.isEmpty(state.errors) && _.isEmpty(state.warnings)) {
                    this.props.onClose();
                } else {
                    this.setState({error_state: state});
                }
            });
    }

    saveArtifact = () => {
        api.post("v1/SetArtifactFile", {
            artifact: this.state.text,
            op: "SET",
        }, this.source.token).then(
            (response) => {
                this.props.onClose();
            });
    }


    reformatArtifact = ()=>{
        this.setState({loading: true});
        api.post("v1/ReformatVQL",
                 {artifact: this.state.text}, this.source.token).then(
                     response=>{
                         if (response.data.artifact) {
                             this.setState({text: response.data.artifact});
                         };
                         this.setState({loading: false});
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
            placeholder: "",
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
                   keyboard={false}
                   scrollable={true}>
              <Modal.Header closeButton
                    onHide={this.props.onClose}>
                <Modal.Title>{ this.props.name ?
                               T("Edit Artifact", this.props.name) :
                               T("Create a new artifact")}
                </Modal.Title>
              </Modal.Header>
              <Modal.Body>
                { this.state.error_state.error &&
                  <SetArtifactErrorDialog
                    error={this.state.error_state}
                    onClose={x=>this.setState({error_state: {}})}
                    onIgnore={this.saveArtifact}
                  /> }

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
                  <Button variant="default"
                          onClick={this.reformatArtifact}>
                    <FontAwesomeIcon icon="indent"/>
                  </Button>
                </ButtonGroup>

                <ButtonGroup className="float-right">
                  <Button variant="default"
                          onClick={this.props.onClose}>
                    <FontAwesomeIcon icon="window-close"/>
                    <span className="button-label">{T("Close")}</span>
                  </Button>
                  <Button variant="primary"
                          onClick={this.checkArtifact}>
                    <FontAwesomeIcon icon="save"/>
                    <span className="button-label">{T("Save")}</span>
                  </Button>
                </ButtonGroup>
                </Navbar>
              </Modal.Footer>
            </Modal>
        );
    }
};
