import React, { Component } from 'react';
import Modal from 'react-bootstrap/Modal';
import Card from 'react-bootstrap/Card';
import Row from 'react-bootstrap/Row';
import Col from 'react-bootstrap/Col';
import Button from 'react-bootstrap/Button';
import logo from  "../sidebar/velo.svg";
import "./login.css";
import api from '../core/api-service.jsx';


export default class LogoffPage extends Component {
    render() {
        let username = window.ErrorState && window.ErrorState.Username;

        return (
            <Modal className="full-height"
                   show={true}
                   dialogClassName="modal-90w"
                   enforceFocus={true}
                   scrollable={true}
                   onHide={() => {
                       window.location = api.href("/app/index.html");
                   }}>
              <Modal.Header closeButton>
                <Modal.Title>Velociraptor Login</Modal.Title>
              </Modal.Header>
              <Modal.Body className="tool-viewer">

                { username &&
                  <Card>
                    <Card.Header>
                      User {username} has now logged off.
                    </Card.Header>
                    <Card.Body>
                      Thank you for using Velociraptor... Digging Deeper!
                    </Card.Body>
                  </Card>
                }
                <div className='login-options'>
                  <Row>
                    <Col sm="3"/>
                    <Col sm="6">
                      <Button
                        variant="outline" className="login">
                        <a href={api.base_path()}>
                          <Row>
                            <Col sm="1">
                              <img src={api.src_of(logo)}
                                   className="velo-logo" alt="velo logo"/>
                            </Col>
                            <Col sm="9" className="provider">
                              <span>
                                Sign in again
                              </span>
                            </Col>
                          </Row>
                        </a>
                      </Button>
                    </Col>
                  </Row>
                </div>
              </Modal.Body>
              <Modal.Footer>
              </Modal.Footer>
            </Modal>
        );
    }
}
