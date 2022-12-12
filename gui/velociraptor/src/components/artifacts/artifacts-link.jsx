import React from 'react';
import PropTypes from 'prop-types';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import VeloReportViewer from "../artifacts/reporting.jsx";
import Modal from 'react-bootstrap/Modal';
import T from '../i8n/i8n.jsx';

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
                    <Modal.Title>{T("Artifact details")}</Modal.Title>
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
                      {T("Close")}
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
