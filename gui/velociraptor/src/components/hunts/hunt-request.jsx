import _ from 'lodash';

import React from 'react';
import PropTypes from 'prop-types';
import T from '../i8n/i8n.jsx';
import Row from 'react-bootstrap/Row';
import Card from 'react-bootstrap/Card';
import VeloAce from '../core/ace.jsx';
import api from '../core/api-service.jsx';
import  {CancelToken} from 'axios';


export default class HuntRequest extends React.Component {
    static propTypes = {
        hunt: PropTypes.object,
    };

    componentDidMount = () => {
        this.source = CancelToken.source();
        let hunt_id = this.props.hunt && this.props.hunt.hunt_id;
        if (hunt_id) {
            this.loadFullHunt(hunt_id);
        }
    }

    componentWillUnmount() {
        this.source.cancel();
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        let prev_hunt_id = prevProps.hunt && prevProps.hunt.hunt_id;
        let current_hunt_id = this.props.hunt && this.props.hunt.hunt_id;
        if (prev_hunt_id !== current_hunt_id) {
            // Re-render the table if the hunt id changes.
            this.loadFullHunt(current_hunt_id);
        };
        return false;
    }

    state = {
        full_selected_hunt: {},
    }

    loadFullHunt = (hunt_id) => {
        this.source.cancel();
        this.source = CancelToken.source();

        api.get("v1/GetHunt", {
            hunt_id: hunt_id,
            include_request: true,
        }, this.source.token).then((response) => {
            if (response.cancel) return;

            if(_.isEmpty(response.data)) {
                this.setState({hunt: {}});
            } else {
                this.setState({hunt: response.data});
            }
        });
    }


    render() {
        let hunt = this.state.hunt || {};
        let serialized = JSON.stringify(hunt.start_request, null, 2);
        let options = {
            readOnly: true,
        };

        return (
            <Row>
              <Card>
                <Card.Header>{T("Request sent to client")}</Card.Header>
                <Card.Body>
                  <VeloAce text={serialized}
                           options={options} />
                </Card.Body>
              </Card>
            </Row>
        );
    }
};
