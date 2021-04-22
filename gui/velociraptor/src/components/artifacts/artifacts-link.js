import React from 'react';
import PropTypes from 'prop-types';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import VeloReportViewer from "../artifacts/reporting.js";
import Modal from 'react-bootstrap/Modal';

export default class ArtifactLink extends React.Component {
    static propTypes = {
        artifact: PropTypes.string,
    };

    state = {
        artifact_definition: {},
        showInspector: false,
    }

    render() {
        return (
            <>
            { this.state.showInspector &&
              <Modal show={true}
                     size="lg"
                     enforceFocus={false}
                     scrollable={true}
                     onHide={() => this.setState({showInspector: false})} >
                  <Modal.Header closeButton>
                    <Modal.Title>Artifact details</Modal.Title>
                  </Modal.Header>
                  <Modal.Body>
                    <VeloReportViewer
                      artifact={this.props.artifact}
                      type="ARTIFACT_DESCRIPTION"
                    />
                  </Modal.Body>
                  <Modal.Footer>
                    <Button variant="secondary"
                            onClick={() => this.setState({
                                showInspector: false,
                            })}>
                      Close
                    </Button>
                  </Modal.Footer>
                </Modal>
            }
              <Button size="sm" className="client-link"
                      onClick={()=>this.setState({
                          showInspector: true,
                      })}
                      variant="outline-info">
                <FontAwesomeIcon icon="external-link-alt"/>
                <span className="button-label">{ this.props.artifact }</span>
              </Button>
            </>
        );
    }
};
