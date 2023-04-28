import _ from 'lodash';

import PropTypes from 'prop-types';
import React, { Component } from 'react';
import Modal from 'react-bootstrap/Modal';
import T from '../i8n/i8n.jsx';
import UserConfig from '../core/user.jsx';
import OrgSelector from '../hunts/orgs.jsx';
import Button from 'react-bootstrap/Button';
import {CancelToken} from 'axios';
import api from '../core/api-service.jsx';


function getOrgsForUser(users, name) {
    if (_.isEmpty(users)) {
        return [];
    }

    for (let i=0; i<users.length; i++) {
        if (users[i].name === name) {
            return _.map(users[i].orgs, x=>x.id);
        }
    }
    return [];
}


export default class AddOrgDialog extends Component {
    static contextType = UserConfig;

    static propTypes = {
        // The selected user we are working on
        user_name: PropTypes.string,

        // All the users from GetGlobalUsers
        users: PropTypes.array,
        onClose: PropTypes.func.isRequired,
    }

    state = {
        orgs: [],
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel();
    }

    addUserToOrg = () => {
        this.source.cancel();
        this.source = CancelToken.source();

        let name = this.props.user_name;
        if(!name) return;

        let existing_orgs = getOrgsForUser(this.props.users, name);
        let req = {
            name: name,
            roles: ["reader"],
            orgs: _.difference(this.state.orgs, existing_orgs || []),
        };

        api.post("v1/CreateUser", req,
                 this.source.token).then(response=>{
                     if (response.cancel)
                         return;
                     this.props.onClose();
                 });
    }

    render() {
        return (
            <Modal show={true}
                   onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>{T("Assign user to Orgs")}</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <OrgSelector
                  onChange={orgs=>this.setState({
                      orgs: orgs,
                  })}
                  value={this.state.orgs} />
              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary"
                        onClick={()=>this.props.onClose()}>
                  {T("Close")}
                </Button>
                <Button variant="primary"
                        onClick={this.addUserToOrg}>
                  {T("Do it!")}
                </Button>
              </Modal.Footer>

            </Modal>
        );
    }
}
