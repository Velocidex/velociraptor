import _ from 'lodash';
import PropTypes from 'prop-types';
import React, { Component } from 'react';
import Modal from 'react-bootstrap/Modal';
import Button from 'react-bootstrap/Button';
import Row from 'react-bootstrap/Row';
import Col from 'react-bootstrap/Col';
import Card from 'react-bootstrap/Card';

import "./login.css";
import github_logo from "./Github-octocat-icon-vector-01.svg";
import google_logo from "./Google-icon-vector-04.svg";
import azure_logo from "./Microsoft_Azure_Logo.svg";
import openid_logo from "./OpenID_logo.svg";


class Authenticator extends Component {
    static propTypes = {
        param: PropTypes.object,
    }

    getIcon = provider=>{
        switch(provider) {
        case "Google":
            return <img className="logo" alt='google logo'
                        src={google_logo}/>;
        case "Github":
            return <img className="logo" alt='github logo'
                        src={github_logo}/>;
        case "Microsoft":
            return <img className="logo" alt='microsoft logo'
                        src={azure_logo}/>;
        case "Generic OIDC connector":
            return <img className="logo" alt='oidc logo'
                        src={openid_logo}/>;
        default:
            return <div/>;
        }
    }

    render() {
        return (
            <Row>
              <Col sm="3"/>
              <Col sm="6">
                <Button
                  variant="outline" className="login">
                  <a href={this.props.param.LoginURL}>
                    <Row>
                    <Col sm="1">
                      {this.getIcon(this.props.param.ProviderName)}
                    </Col>
                      <Col sm="9" className="provider">
                        <span>
                          Sign in with {this.props.param.ProviderName}
                        </span>
                      </Col>
                    </Row>
                  </a>
                </Button>
              </Col>
            </Row>
        );
    }
};

export default class LoginPage extends Component {
    render() {
        let authenticators = (window.ErrorState &&
                              window.ErrorState.Authenticators) || [];
        let username = window.ErrorState && window.ErrorState.Username;
        return (
            <Modal className="full-height"
                   show={true}
                   dialogClassName="modal-90w"
                   enforceFocus={true}
                   scrollable={true}
                   onHide={() => {
                       window.location = '/';
                   }}>
              <Modal.Header closeButton>
                <Modal.Title>Velociraptor Login</Modal.Title>
              </Modal.Header>
              <Modal.Body className="tool-viewer">

                { username &&
                  <Card>
                    <Card.Header>
                      User {username} is not registered on this system
                    </Card.Header>
                    <Card.Body>
                      Please contact your system administrator to be
                      added to this system, or log in with
                      a different user account.
                    </Card.Body>
                  </Card>
                }

                <div className='login-options'>
                  {_.map(authenticators, x=>{
                      return <Authenticator param={x}/>;
                  })}
                </div>
              </Modal.Body>
              <Modal.Footer>
              </Modal.Footer>
            </Modal>
        );
    }
}
