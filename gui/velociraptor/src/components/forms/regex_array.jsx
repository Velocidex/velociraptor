import _ from 'lodash';
import PropTypes from 'prop-types';
import React, { Component } from 'react';
import RegEx from './regex.jsx';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { JSONparse } from '../utils/json_parse.jsx';


export default class RegExArray extends Component {
    static propTypes = {
        value: PropTypes.string,
        setValue: PropTypes.func.isRequired,
    };

    addNewRegex = idx=>{
        let regex_array = this.getRegexArray();
        regex_array.splice(idx, 0, ".");
        this.props.setValue(JSON.stringify(regex_array));
    }

    removeRegex = idx=>{
        let regex_array = this.getRegexArray();
        regex_array.splice(idx, 1);
        this.props.setValue(JSON.stringify(regex_array));
    }

    setValue = (x, idx)=>{
        let regex_array = this.getRegexArray();
        if (idx >= 0 && idx < regex_array.length) {
            regex_array.splice(idx, 1, x);
        }
        this.props.setValue(JSON.stringify(regex_array));
    }

    getRegexArray = ()=>JSONparse(this.props.value, []);

    render() {
        return (
            <div>
              <ButtonGroup>
                <Button variant="default-outline" size="sm"
                        onClick={(e)=>{
                            this.addNewRegex(0);
                            e.preventDefault();
                            return false;
                        }}>
                  <FontAwesomeIcon icon="plus"/>
                </Button>
              </ButtonGroup>

              {_.map(this.getRegexArray(), (x, idx)=>{
                  return <div key={idx} className="regex_array_item">
                           <Button variant="default-outline" size="sm"
                                   className="left-joined-btn"
                                   onClick={(e)=>{
                                       this.removeRegex(idx);
                                       e.preventDefault();
                                       return false;
                                   }}>
                             <FontAwesomeIcon icon="trash"/>
                           </Button>
                           <RegEx key={idx}
                                  value={x}
                                  className="regex_array_item"
                                  setValue={new_x=>this.setValue(new_x, idx)}
                           />
                         </div>;
              })}
            </div>
        );
    }
}
